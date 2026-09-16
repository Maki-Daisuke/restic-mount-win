package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/winfsp/cgofuse/fuse"
)

// openFileInfo tracks an opened file for Read operations.
type openFileInfo struct {
	node    *data.Node
	cumsize []uint64 // cumsize[i] is sum of blob sizes before i
}

// ResticFS implements the cgofuse fuse.FileSystemInterface.
type ResticFS struct {
	fuse.FileSystemBase

	repo       *repository.Repository
	ctx        context.Context
	snapshotID string
	snapshot   *data.Snapshot

	// Snapshots cache for multi-snapshot mount mode
	mu           sync.RWMutex
	snapshots    []*data.Snapshot
	snByID       map[string]*data.Snapshot // short and full IDs
	snByName     map[string]*data.Snapshot // YYYY-MM-DDTHH-MM-SS_id
	snByHost     map[string][]*data.Snapshot
	snByTag      map[string][]*data.Snapshot
	treeCache    map[restic.ID][]*data.Node
	openFiles    map[uint64]*openFileInfo
	handleCount  uint64
	mountTime    time.Time
}

// NewResticFS creates an initialized ResticFS instance.
func NewResticFS(ctx context.Context, repo *repository.Repository, snapshotID string) (*ResticFS, error) {
	fs := &ResticFS{
		repo:        repo,
		ctx:         ctx,
		snapshotID:  snapshotID,
		snByID:      make(map[string]*data.Snapshot),
		snByName:    make(map[string]*data.Snapshot),
		snByHost:    make(map[string][]*data.Snapshot),
		snByTag:     make(map[string][]*data.Snapshot),
		treeCache:   make(map[restic.ID][]*data.Node),
		openFiles:   make(map[uint64]*openFileInfo),
		mountTime:   time.Now(),
	}

	if snapshotID != "" {
		sn, _, err := data.FindSnapshot(ctx, repo, repo, snapshotID)
		if err != nil {
			return nil, fmt.Errorf("finding snapshot %q: %w", snapshotID, err)
		}
		fs.snapshot = sn
	} else {
		if err := fs.refreshSnapshots(); err != nil {
			return nil, fmt.Errorf("loading snapshots: %w", err)
		}
	}

	return fs, nil
}

func (fs *ResticFS) refreshSnapshots() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.snapshots = make([]*data.Snapshot, 0)
	fs.snByID = make(map[string]*data.Snapshot)
	fs.snByName = make(map[string]*data.Snapshot)
	fs.snByHost = make(map[string][]*data.Snapshot)
	fs.snByTag = make(map[string][]*data.Snapshot)

	err := data.ForAllSnapshots(fs.ctx, fs.repo, fs.repo, nil, func(id restic.ID, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		fs.snapshots = append(fs.snapshots, sn)

		fullID := id.String()
		shortID := id.Str()
		fs.snByID[fullID] = sn
		fs.snByID[shortID] = sn

		// Name formatted as: 2006-01-02T15-04-05_<shortID> (colons replaced with dashes for Windows compatibility)
		timeStr := sn.Time.Format("2006-01-02T15-04-05")
		name := fmt.Sprintf("%s_%s", timeStr, shortID)
		fs.snByName[name] = sn

		if sn.Hostname != "" {
			fs.snByHost[sn.Hostname] = append(fs.snByHost[sn.Hostname], sn)
		}
		for _, tag := range sn.Tags {
			fs.snByTag[tag] = append(fs.snByTag[tag], sn)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Sort snapshots latest first
	sort.Slice(fs.snapshots, func(i, j int) bool {
		return fs.snapshots[i].Time.After(fs.snapshots[j].Time)
	})

	if len(fs.snapshots) > 0 {
		fs.snByName["latest"] = fs.snapshots[0]
	}

	return nil
}

func (fs *ResticFS) loadTreeNodes(treeID restic.ID) ([]*data.Node, error) {
	fs.mu.RLock()
	if cached, ok := fs.treeCache[treeID]; ok {
		fs.mu.RUnlock()
		return cached, nil
	}
	fs.mu.RUnlock()

	treeIter, err := data.LoadTree(fs.ctx, fs.repo, treeID)
	if err != nil {
		return nil, fmt.Errorf("loading tree %s: %w", treeID.Str(), err)
	}

	nodes := make([]*data.Node, 0)
	for item := range treeIter {
		if item.Error != nil {
			return nil, fmt.Errorf("reading tree node: %w", item.Error)
		}
		nodes = append(nodes, item.Node)
	}

	fs.mu.Lock()
	fs.treeCache[treeID] = nodes
	fs.mu.Unlock()

	return nodes, nil
}

// traverseTree navigates from rootTreeID through the components of pathParts.
// Returns the target node (or nil for root), remaining path components, and error status code.
func (fs *ResticFS) traverseTree(rootTreeID restic.ID, pathParts []string) (*data.Node, int) {
	currentTreeID := rootTreeID
	var currentNode *data.Node

	for i, part := range pathParts {
		if invalid, _ := IsInvalidWindowsName(part); invalid {
			return nil, -fuse.EINVAL
		}

		nodes, err := fs.loadTreeNodes(currentTreeID)
		if err != nil {
			return nil, -fuse.EIO
		}

		filterResult := FilterDirectoryEntries(nodes)
		resolvedNode := filterResult.ResolveEntry(part)
		if resolvedNode == nil {
			return nil, -fuse.ENOENT
		}

		currentNode = resolvedNode
		if i < len(pathParts)-1 {
			if resolvedNode.Type != data.NodeTypeDir || resolvedNode.Subtree == nil {
				return nil, -fuse.ENOTDIR
			}
			currentTreeID = *resolvedNode.Subtree
		}
	}

	return currentNode, 0
}

// resolvePath resolves a filesystem path into either a restic.Node or a virtual directory.
func (fs *ResticFS) resolvePath(p string) (*data.Node, int) {
	cleanPath := path.Clean("/" + p)
	if cleanPath == "/" {
		// Root directory
		return &data.Node{
			Type:    data.NodeTypeDir,
			Mode:    os.ModeDir | 0555,
			ModTime: fs.mountTime,
		}, 0
	}

	parts := strings.Split(strings.Trim(cleanPath, "/"), "/")

	if fs.snapshot != nil {
		// Single snapshot mount
		if fs.snapshot.Tree == nil {
			return nil, -fuse.EIO
		}
		return fs.traverseTree(*fs.snapshot.Tree, parts)
	}

	// Multi-snapshot mount: virtual top-level directories: snapshots, ids, hosts, tags
	top := parts[0]
	if len(parts) == 1 {
		switch top {
		case "snapshots", "ids", "hosts", "tags":
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: fs.mountTime,
			}, 0
		default:
			return nil, -fuse.ENOENT
		}
	}

	switch top {
	case "snapshots":
		fs.mu.RLock()
		sn := fs.snByName[parts[1]]
		fs.mu.RUnlock()
		if sn == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 2 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: sn.Time,
				Subtree: sn.Tree,
			}, 0
		}
		if sn.Tree == nil {
			return nil, -fuse.EIO
		}
		return fs.traverseTree(*sn.Tree, parts[2:])

	case "ids":
		fs.mu.RLock()
		sn := fs.snByID[parts[1]]
		fs.mu.RUnlock()
		if sn == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 2 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: sn.Time,
				Subtree: sn.Tree,
			}, 0
		}
		if sn.Tree == nil {
			return nil, -fuse.EIO
		}
		return fs.traverseTree(*sn.Tree, parts[2:])

	case "hosts":
		fs.mu.RLock()
		snList := fs.snByHost[parts[1]]
		fs.mu.RUnlock()
		if snList == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 2 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: fs.mountTime,
			}, 0
		}
		// parts[2] is snapshot time/id
		var sn *data.Snapshot
		for _, s := range snList {
			if s.ID().Str() == parts[2] || s.Time.Format("2006-01-02T15-04-05") == parts[2] {
				sn = s
				break
			}
		}
		if sn == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 3 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: sn.Time,
				Subtree: sn.Tree,
			}, 0
		}
		if sn.Tree == nil {
			return nil, -fuse.EIO
		}
		return fs.traverseTree(*sn.Tree, parts[3:])

	case "tags":
		fs.mu.RLock()
		snList := fs.snByTag[parts[1]]
		fs.mu.RUnlock()
		if snList == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 2 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: fs.mountTime,
			}, 0
		}
		var sn *data.Snapshot
		for _, s := range snList {
			if s.ID().Str() == parts[2] || s.Time.Format("2006-01-02T15-04-05") == parts[2] {
				sn = s
				break
			}
		}
		if sn == nil {
			return nil, -fuse.ENOENT
		}
		if len(parts) == 3 {
			return &data.Node{
				Type:    data.NodeTypeDir,
				Mode:    os.ModeDir | 0555,
				ModTime: sn.Time,
				Subtree: sn.Tree,
			}, 0
		}
		if sn.Tree == nil {
			return nil, -fuse.EIO
		}
		return fs.traverseTree(*sn.Tree, parts[3:])
	}

	return nil, -fuse.ENOENT
}

func fillStatFromNode(node *data.Node, stat *fuse.Stat_t) {
	switch node.Type {
	case data.NodeTypeDir:
		stat.Mode = fuse.S_IFDIR | 0555
	case data.NodeTypeSymlink:
		stat.Mode = fuse.S_IFLNK | 0555
	default:
		stat.Mode = fuse.S_IFREG | 0444
	}

	stat.Size = int64(node.Size)
	stat.Mtim = fuse.NewTimespec(node.ModTime)
	stat.Atim = fuse.NewTimespec(node.AccessTime)
	stat.Ctim = fuse.NewTimespec(node.ChangeTime)
	stat.Nlink = 1
}

// Getattr retrieves file attributes.
func (fs *ResticFS) Getattr(p string, stat *fuse.Stat_t, fh uint64) int {
	node, status := fs.resolvePath(p)
	if status != 0 {
		return status
	}
	fillStatFromNode(node, stat)
	return 0
}

// Readdir lists entries in a directory.
func (fs *ResticFS) Readdir(p string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	cleanPath := path.Clean("/" + p)

	fill(".", nil, 0)
	fill("..", nil, 0)

	// In single snapshot mode:
	if fs.snapshot != nil {
		if fs.snapshot.Tree == nil {
			return -fuse.EIO
		}
		var currentTreeID restic.ID
		if cleanPath == "/" {
			currentTreeID = *fs.snapshot.Tree
		} else {
			parts := strings.Split(strings.Trim(cleanPath, "/"), "/")
			node, status := fs.traverseTree(*fs.snapshot.Tree, parts)
			if status != 0 {
				return status
			}
			if node.Type != data.NodeTypeDir || node.Subtree == nil {
				return -fuse.ENOTDIR
			}
			currentTreeID = *node.Subtree
		}

		nodes, err := fs.loadTreeNodes(currentTreeID)
		if err != nil {
			return -fuse.EIO
		}

		result := FilterDirectoryEntries(nodes)
		// Print warnings on every directory operation as specified
		PrintFilterWarnings(os.Stderr, cleanPath, result)

		for _, entry := range result.VisibleEntries {
			var st fuse.Stat_t
			fillStatFromNode(entry, &st)
			if !fill(entry.Name, &st, 0) {
				break
			}
		}
		return 0
	}

	// In multi-snapshot mode:
	if cleanPath == "/" {
		topDirs := []string{"snapshots", "ids", "hosts", "tags"}
		for _, name := range topDirs {
			st := fuse.Stat_t{
				Mode: fuse.S_IFDIR | 0555,
				Mtim: fuse.NewTimespec(fs.mountTime),
			}
			if !fill(name, &st, 0) {
				break
			}
		}
		return 0
	}

	parts := strings.Split(strings.Trim(cleanPath, "/"), "/")
	top := parts[0]

	if len(parts) == 1 {
		fs.mu.RLock()
		defer fs.mu.RUnlock()

		switch top {
		case "snapshots":
			for name, sn := range fs.snByName {
				st := fuse.Stat_t{
					Mode: fuse.S_IFDIR | 0555,
					Mtim: fuse.NewTimespec(sn.Time),
				}
				if !fill(name, &st, 0) {
					break
				}
			}
			return 0

		case "ids":
			seen := make(map[string]struct{})
			for _, sn := range fs.snapshots {
				shortID := sn.ID().Str()
				if _, ok := seen[shortID]; !ok {
					seen[shortID] = struct{}{}
					st := fuse.Stat_t{
						Mode: fuse.S_IFDIR | 0555,
						Mtim: fuse.NewTimespec(sn.Time),
					}
					if !fill(shortID, &st, 0) {
						break
					}
				}
			}
			return 0

		case "hosts":
			for host := range fs.snByHost {
				st := fuse.Stat_t{
					Mode: fuse.S_IFDIR | 0555,
					Mtim: fuse.NewTimespec(fs.mountTime),
				}
				if !fill(host, &st, 0) {
					break
				}
			}
			return 0

		case "tags":
			for tag := range fs.snByTag {
				st := fuse.Stat_t{
					Mode: fuse.S_IFDIR | 0555,
					Mtim: fuse.NewTimespec(fs.mountTime),
				}
				if !fill(tag, &st, 0) {
					break
				}
			}
			return 0
		}
	}

	// If inside a snapshot tree (e.g. /snapshots/<name>/... or /ids/<id>/...)
	node, status := fs.resolvePath(p)
	if status != 0 {
		return status
	}
	if node.Type != data.NodeTypeDir || node.Subtree == nil {
		return -fuse.ENOTDIR
	}

	nodes, err := fs.loadTreeNodes(*node.Subtree)
	if err != nil {
		return -fuse.EIO
	}

	result := FilterDirectoryEntries(nodes)
	PrintFilterWarnings(os.Stderr, cleanPath, result)

	for _, entry := range result.VisibleEntries {
		var st fuse.Stat_t
		fillStatFromNode(entry, &st)
		if !fill(entry.Name, &st, 0) {
			break
		}
	}
	return 0
}

// Open opens a file for reading.
func (fs *ResticFS) Open(p string, flags int) (int, uint64) {
	// Refuse write access (read-only filesystem)
	if flags&(fuse.O_WRONLY|fuse.O_RDWR|fuse.O_CREAT|fuse.O_TRUNC) != 0 {
		return -fuse.EROFS, ^uint64(0)
	}

	node, status := fs.resolvePath(p)
	if status != 0 {
		return status, ^uint64(0)
	}
	if node.Type == data.NodeTypeDir {
		return -fuse.EISDIR, ^uint64(0)
	}

	// Calculate cumulative size of blobs
	var totalBytes uint64
	cumsize := make([]uint64, 1+len(node.Content))
	for i, blobID := range node.Content {
		size, found := fs.repo.LookupBlobSize(restic.BlobHandle{Type: restic.DataBlob, ID: blobID})
		if !found {
			return -fuse.EIO, ^uint64(0)
		}
		totalBytes += uint64(size)
		cumsize[i+1] = totalBytes
	}

	fh := atomic.AddUint64(&fs.handleCount, 1)

	fs.mu.Lock()
	fs.openFiles[fh] = &openFileInfo{
		node:    node,
		cumsize: cumsize,
	}
	fs.mu.Unlock()

	return 0, fh
}

// Read reads data from an opened file.
func (fs *ResticFS) Read(p string, buff []byte, ofst int64, fh uint64) int {
	fs.mu.RLock()
	info, ok := fs.openFiles[fh]
	fs.mu.RUnlock()
	if !ok {
		return -fuse.EBADF
	}

	if ofst < 0 {
		return -fuse.EINVAL
	}

	fileSize := int64(info.node.Size)
	if ofst >= fileSize || len(buff) == 0 {
		return 0
	}

	reqSize := len(buff)
	if ofst+int64(reqSize) > fileSize {
		reqSize = int(fileSize - ofst)
	}

	offset := uint64(ofst)
	// Find starting blob
	startBlobIdx := -1 + sort.Search(len(info.cumsize), func(i int) bool {
		return info.cumsize[i] > offset
	})
	if startBlobIdx < 0 || startBlobIdx >= len(info.node.Content) {
		return 0
	}

	offsetInBlob := offset - info.cumsize[startBlobIdx]
	bytesRead := 0
	remaining := reqSize

	for i := startBlobIdx; remaining > 0 && i < len(info.node.Content); i++ {
		blobID := info.node.Content[i]
		blobData, err := fs.repo.LoadBlob(fs.ctx, restic.BlobHandle{Type: restic.DataBlob, ID: blobID}, nil)
		if err != nil {
			return -fuse.EIO
		}

		if offsetInBlob > 0 {
			if offsetInBlob < uint64(len(blobData)) {
				blobData = blobData[offsetInBlob:]
			} else {
				blobData = nil
			}
			offsetInBlob = 0
		}

		n := copy(buff[bytesRead:], blobData)
		bytesRead += n
		remaining -= n
	}

	return bytesRead
}

// Release closes an open file handle.
func (fs *ResticFS) Release(p string, fh uint64) int {
	fs.mu.Lock()
	delete(fs.openFiles, fh)
	fs.mu.Unlock()
	return 0
}

// Readlink reads the target of a symbolic link.
func (fs *ResticFS) Readlink(p string) (int, string) {
	node, status := fs.resolvePath(p)
	if status != 0 {
		return status, ""
	}
	if node.Type != data.NodeTypeSymlink {
		return -fuse.EINVAL, ""
	}
	return 0, node.LinkTarget
}

// Statfs returns filesystem statistics.
func (fs *ResticFS) Statfs(p string, stat *fuse.Statfs_t) int {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 100000000
	stat.Bfree = 100000000
	stat.Bavail = 100000000
	stat.Files = 100000000
	stat.Ffree = 100000000
	stat.Favail = 100000000
	stat.Namemax = 255
	return 0
}
