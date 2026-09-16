package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"

	"github.com/restic/restic/internal/backend/all"
	"github.com/restic/restic/internal/backend/local"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"
)

type MountWinOptions struct {
	SnapshotID string
}

func main() {
	// Pre-initialize WinFsp FUSE layer early at process startup to ensure the WinFsp DLL
	// is properly located and initialized before any network or backend operations.
	_, _ = fuse.OptParse([]string{"-o", "ro"}, "")

	globalOptions := global.Options{
		Backends: all.Backends(),
	}

	var mountOpts MountWinOptions

	rootCmd := &cobra.Command{
		Use:   "restic-mount [flags] mountpoint",
		Short: "Mount a restic repository as a Windows virtual drive or folder",
		Long: `restic-mount mounts a restic repository read-only on Windows using WinFsp.

You can specify a drive letter (e.g. X:) or a folder path (e.g. C:\mnt\restic).
To mount a specific snapshot directly at the root of the mountpoint, pass --snapshot <ID>.
`,
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			return globalOptions.PreRun(true)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.Fatal("missing mountpoint argument (e.g. X: or C:\\mnt\\restic)")
			}
			mountpoint := args[0]

			// Validate repository location
			if globalOptions.Repo == "" && globalOptions.RepositoryFile == "" {
				return errors.Fatal("no repository specified, please specify repository location via -r or $RESTIC_REPOSITORY")
			}

			// Validate mountpoint and repo overlap (deadlock avoidance)
			loc, err := location.Parse(globalOptions.Backends, globalOptions.Repo)
			if err == nil && loc.Scheme == "local" {
				if localCfg, ok := loc.Config.(*local.Config); ok {
					if err := CheckMountpointOverlap(localCfg.Path, mountpoint); err != nil {
						return err
					}
				}
			}

			term, cancelTerm := termstatus.Setup(os.Stdin, os.Stdout, os.Stderr, globalOptions.Quiet)
			defer cancelTerm()
			globalOptions.Term = term

			printer := progress.NewTerminalPrinter(false, globalOptions.Verbosity, term)

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Open repository with shared lock
			ctx, repo, unlock, err := OpenWithReadLock(ctx, globalOptions, globalOptions.NoLock, printer)
			if err != nil {
				return err
			}
			defer unlock()

			printer.P("Loading repository index...\n")
			if err := repo.LoadIndex(ctx, printer); err != nil {
				return fmt.Errorf("loading repository index: %w", err)
			}

			rfs, err := NewResticFS(ctx, repo, mountOpts.SnapshotID)
			if err != nil {
				return fmt.Errorf("initializing virtual filesystem: %w", err)
			}

			host := fuse.NewFileSystemHost(rfs)

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				printer.P("\nReceived interrupt, unmounting %s...\n", mountpoint)
				host.Unmount()
				cancel()
			}()

			printer.P("Now serving the repository at %s\n", mountpoint)
			printer.P("Use another terminal or File Explorer to browse the contents.\n")
			printer.P("When finished, quit with Ctrl-c here to unmount %s.\n", mountpoint)

			mountOptions := []string{
				"-o", "ro",
			}
			if !host.Mount(mountpoint, mountOptions) {
				return errors.Fatal(fmt.Sprintf("failed to mount at %s (please ensure WinFsp is installed and the mountpoint is available)", mountpoint))
			}

			return nil
		},
	}

	globalOptions.AddFlags(rootCmd.Flags())
	rootCmd.Flags().StringVar(&mountOpts.SnapshotID, "snapshot", "", "mount only the specified snapshot ID directly at the root")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
}
