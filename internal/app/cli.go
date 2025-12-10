package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ghostlawless/xdl/internal/log"
)

type RunContext struct {
	Users             []string
	Mode              RunMode
	RunID             string
	RunSeed           []byte
	LogPath           string
	CookiePath        string
	CookiePersistPath string
	OutRoot           string
	NoDownload        bool
	DryRun            bool
}

type RunMode int

func Run() {
	if err := RunWithArgsAndID(os.Args[1:], "", nil); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
	}
}

func RunWithArgs(args []string) error {
	return RunWithArgsAndID(args, "", nil)
}

func RunWithArgsAndID(args []string, runID string, runSeed []byte) error {
	rctx, err := parseArgs(args, runID, runSeed)
	if err != nil {
		return err
	}
	return runWithContext(rctx)
}

func parseArgs(args []string, presetRunID string, presetRunSeed []byte) (RunContext, error) {
	var (
		fQuiet             bool
		fDebug             bool
		fCookiePath        string
		fCookiePersistPath string
	)
	for _, a := range args {
		switch a {
		case "-q", "/q":
			fQuiet = true
		case "-d", "/d":
			fDebug = true
		}
	}
	fs := flag.NewFlagSet("xdl", flag.ContinueOnError)
	fs.BoolVar(&fQuiet, "q", fQuiet, "Quiet mode")
	fs.BoolVar(&fDebug, "d", fDebug, "Debug mode")
	if err := fs.Parse(args); err != nil {
		return RunContext{}, err
	}
	rest := fs.Args()
	if (len(rest) == 0 || rest[0] == "") && fCookiePersistPath != "" {
		ctx := RunContext{
			Users:             nil,
			Mode:              ModeVerbose,
			RunID:             presetRunID,
			RunSeed:           presetRunSeed,
			CookiePath:        "",
			CookiePersistPath: fCookiePersistPath,
			OutRoot:           "xDownloads",
			NoDownload:        true,
			DryRun:            false,
		}
		if fDebug {
			ctx.Mode = ModeDebug
		} else if fQuiet {
			ctx.Mode = ModeQuiet
		}
		if ctx.RunID == "" {
			ctx.RunID = generateRunID()
		}
		if ctx.Mode == ModeDebug {
			ctx.LogPath = filepath.Join("logs", "run_"+ctx.Users[0]+"_"+ctx.RunID)
			if err := os.MkdirAll(ctx.LogPath, 0o755); err != nil {
				return RunContext{}, fmt.Errorf("failed to create log dir: %w", err)
			}
			log.Init(filepath.Join(ctx.LogPath, "main.log"))
			log.LogInfo("main", "Debug mode enabled; logs stored in "+ctx.LogPath)
		} else {
			log.Disable()
		}
		return ctx, nil
	}
	users := make([]string, 0, len(rest))
	for _, u := range rest {
		if u == "" {
			continue
		}
		if u == "-d" || u == "/d" || u == "-q" || u == "/q" {
			continue
		}
		users = append(users, u)
	}
	if len(users) == 0 {
		return RunContext{}, fmt.Errorf("usage: xdl [-q|-d] <username> [more_usernames...]")
	}
	ctx := RunContext{
		Users:             users,
		Mode:              ModeVerbose,
		RunID:             presetRunID,
		RunSeed:           presetRunSeed,
		CookiePath:        fCookiePath,
		CookiePersistPath: fCookiePersistPath,
		OutRoot:           "xDownloads",
		NoDownload:        false,
		DryRun:            false,
	}
	if fDebug {
		ctx.Mode = ModeDebug
	} else if fQuiet {
		ctx.Mode = ModeQuiet
	}
	if ctx.RunID == "" {
		ctx.RunID = generateRunID()
	}
	if ctx.Mode == ModeDebug {
		ctx.LogPath = filepath.Join("logs", "run_"+ctx.RunID)
		if err := os.MkdirAll(ctx.LogPath, 0o755); err != nil {
			return RunContext{}, fmt.Errorf("failed to create log dir: %w", err)
		}
		log.Init(filepath.Join(ctx.LogPath, "main.log"))
		log.LogInfo("main", "Debug mode enabled; logs stored in "+ctx.LogPath)
	} else {
		log.Disable()
	}
	return ctx, nil
}
