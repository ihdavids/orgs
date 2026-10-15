package commands

// Printing an html page to pdf with a headless Chrome on this machine.
//
// The server writes the page and this side prints it, because the browser is
// on the machine the person is at and the server may be a box with no screen
// and no Chrome. The same copy as orgs invoice's (cmd/oc/commands/invoice),
// lifted out so orgs print need not have its own; invoice can switch to it.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// FindBrowser is a browser that can print a page to pdf without a window, in
// the order they are tried, or "". ORGS_CHROME names one outright.
func FindBrowser() string {
	if p := os.Getenv("ORGS_CHROME"); p != "" {
		return p
	}
	for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
		"microsoft-edge", "msedge", "brave-browser", "brave"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	cands := []string{}
	switch runtime.GOOS {
	case "darwin":
		for _, app := range []string{"Google Chrome", "Chromium", "Microsoft Edge", "Brave Browser", "Google Chrome Canary"} {
			cands = append(cands, "/Applications/"+app+".app/Contents/MacOS/"+app)
			if home, err := os.UserHomeDir(); err == nil {
				cands = append(cands, filepath.Join(home, "Applications", app+".app/Contents/MacOS", app))
			}
		}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			cands = append(cands, filepath.Join(base, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// PrintPDF prints a self-contained html page to pdf. The page's own @page
// rules say the paper size and margins; Chrome's header and footer are off.
func PrintPDF(html []byte) ([]byte, error) {
	browser := FindBrowser()
	if browser == "" {
		return nil, fmt.Errorf("making a pdf needs Chrome, Chromium or Edge (or ORGS_CHROME=/path/to/one)")
	}
	dir, err := os.MkdirTemp("", "orgs-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	page := filepath.Join(dir, "page.html")
	pdf := filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(page, html, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Its own profile, so a Chrome already open is neither disturbed nor
	// handed the job (which would return at once with no pdf).
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--disable-gpu", "--no-first-run",
		"--no-default-browser-check", "--disable-extensions", "--user-data-dir="+filepath.Join(dir, "profile"),
		"--no-pdf-header-footer", "--print-to-pdf-no-header", "--print-to-pdf="+pdf, "file://"+filepath.ToSlash(page))
	out, err := cmd.CombinedOutput()
	b, rerr := os.ReadFile(pdf)
	if rerr != nil || len(b) == 0 {
		if err == nil {
			err = rerr
		}
		return nil, fmt.Errorf("%s did not make a pdf: %v\n%s", filepath.Base(browser), err, strings.TrimSpace(string(out)))
	}
	return b, nil
}
