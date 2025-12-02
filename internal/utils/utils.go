package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	xlog "github.com/ghostlawless/xdl/internal/log"
)

const (
	cR  = "\033[0m"
	cCy = "\033[36;1m"
	cGn = "\033[32;1m"
	cYl = "\033[33;1m"
	cRd = "\033[31;1m"
)

var srep = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

func pf(c string) string { return c + "xdl ▸" + cR }

func PrintInfo(f string, a ...any) { fmt.Fprintf(os.Stdout, "%s %s\n", pf(cCy), fmt.Sprintf(f, a...)) }
func PrintSuccess(f string, a ...any) {
	fmt.Fprintf(os.Stdout, "%s %s\n", pf(cGn), fmt.Sprintf(f, a...))
}
func PrintWarn(f string, a ...any)  { fmt.Fprintf(os.Stderr, "%s %s\n", pf(cYl), fmt.Sprintf(f, a...)) }
func PrintError(f string, a ...any) { fmt.Fprintf(os.Stderr, "%s %s\n", pf(cRd), fmt.Sprintf(f, a...)) }

func PrintBanner() {
	const b = `
           /$$$$$$$  
          | $$__  $$ 
 /$$   /$$| $$  \ $$
|  $$ /$$/| $$  | $$
 \  $$$$/ | $$  | $$
  >$$  $$ | $$  | $$
 /$$/\  $$| $$$$$$$/
|__/  \__/|_______/ 

xdl ▸ x Downloader
`
	fmt.Fprintf(os.Stdout, "%s%s%s\n", cCy, b, cR)
}

func EnsureDir(p string) error {
	if p == "" {
		return fmt.Errorf("empty dir")
	}
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return nil
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		xlog.LogError("utils.ensure_dir", err.Error())
		return err
	}
	return nil
}

func DirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func SanitizeFilename(name string) string {
	if name == "" {
		return "file"
	}
	name = srep.Replace(filepath.Base(name))
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	return strings.TrimRight(name, ". ")
}

func SaveToFile(p string, data []byte) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if err := EnsureDir(filepath.Dir(p)); err != nil {
		return err
	}
	base := filepath.Base(p)
	tmp, err := os.CreateTemp(filepath.Dir(p), base+".tmp-*")
	if err != nil {
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	tp := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	if _, err := os.Stat(p); err == nil {
		_ = os.Remove(p)
	}
	if err := os.Rename(tp, p); err == nil {
		return nil
	}
	in, err := os.Open(tp)
	if err != nil {
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		in.Close()
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		in.Close()
		_ = os.Remove(p)
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	if err := out.Close(); err != nil {
		in.Close()
		_ = os.Remove(p)
		_ = os.Remove(tp)
		xlog.LogError("utils.save_file", err.Error())
		return err
	}
	in.Close()
	_ = os.Remove(tp)
	return nil
}

func SaveText(p string, s string) error {
	return SaveToFile(p, []byte(s))
}

func SaveTimestamped(dir, pref, ext string, data []byte) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("empty baseDir")
	}
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405.000000000")
	sfx := randHex(4)
	pref = SanitizeFilename(pref)
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "bin"
	}
	name := fmt.Sprintf("%s_%s_%s.%s", pref, ts, sfx, ext)
	full := filepath.Join(dir, name)
	if err := SaveToFile(full, data); err != nil {
		return "", err
	}
	xlog.LogInfo("utils.save_ts", "saved: "+full)
	return full, nil
}

func SaveJSONDebug(dir, name string, b []byte) {
	if dir == "" || name == "" {
		xlog.LogError("utils.save_json_debug", "invalid baseDir/name")
		return
	}
	if err := EnsureDir(dir); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}
	name = SanitizeFilename(name)
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	p := filepath.Join(dir, name)
	if err := SaveToFile(p, b); err != nil {
		xlog.LogError("utils.save_json_debug", err.Error())
		return
	}
	xlog.LogInfo("debug", "saved: "+p)
}

func PromptYesNoDefaultYes(q string) bool {
	fmt.Fprint(os.Stdout, q)
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "" || line == "y" || line == "yes"
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (uint(i) & 7))
		}
	}
	return hex.EncodeToString(b)
}
