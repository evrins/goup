package commands

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/owenthereal/goup/internal/entity"
	"github.com/owenthereal/goup/internal/service"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

const (
	goHost                = "golang.google.cn"
	goSourceGitURL        = "https://github.com/golang/go"
	goSourceUpsteamGitURL = "https://go.googlesource.com/go"
)

var (
	installCmdGoHostFlag string
)

func GetGoSourceGitURL() string {
	gsURL := os.Getenv("GOUP_GO_SOURCE_GIT_URL")
	if gsURL == "" {
		gsURL = goSourceGitURL
	}
	return gsURL
}

func GetGoHost() string {
	gh := os.Getenv("GOUP_GO_HOST")
	if gh == "" {
		gh = goHost
	}
	return gh
}

func getGoArch() string {
	if arch := os.Getenv("GOUP_GO_ARCH"); arch != "" {
		return arch
	}
	return runtime.GOARCH
}

// getOS returns runtime.GOOS. It exists as a function just for lazy
// testing of the Windows zip path when running on Linux/Darwin.
func getOS() string {
	return runtime.GOOS
}

func installCmd() *cobra.Command {
	installCmd := &cobra.Command{
		Use:   "install [VERSION]",
		Short: `Install Go with a version`,
		Long: `Install Go by providing a version. If no version is provided, install
the latest Go. If the version is 'tip', an optional change list (CL)
number can be provided.`,
		Example: `
  goup install
  goup install 1.15.2
  goup install go1.15.2
  goup install tip # Compile Go tip
  goup install tip 1234 # 1234 is the CL number
`,
		RunE: runInstall,
	}

	installCmd.PersistentFlags().StringVar(&installCmdGoHostFlag, "host", GetGoHost(), "host that is used to download Go. The GOUP_GO_HOST environment variable overrides this flag.")

	return installCmd
}

func runInstall(cmd *cobra.Command, args []string) (err error) {
	var release entity.Release
	var version string
	var svc = service.NewGoReleaseService(GetGoHost())

	if len(args) == 0 {
		release, err = svc.GetLatestRelease()
		if err != nil {
			return err
		}
		version = release.Version
	} else {
		version = args[0]
		if version == "tip" {
			var cl string
			if len(args) > 1 {
				cl = args[1]
			}
			err = installTip(cl)
		} else {
			var rl2 entity.ReleaseList
			rl2, err = svc.GetReleaseWithFilter(version)
			if err != nil {
				return
			}
			if rl2.Len() == 0 {
				err = errors.New("no matched go version found")
				return
			}
			release = rl2[0]
			err = install(release)
			version = release.Version
		}
	}

	if err != nil {
		return err
	}

	if err := switchVer(version); err != nil {
		return err
	}

	return nil
}

func switchVer(ver string) error {
	if !strings.HasPrefix(ver, "go") {
		ver = "go" + ver
	}

	err := symlink(ver)

	if err == nil {
		logger.Printf("Default Go is set to '%s'", ver)
	}

	return err
}

func symlink(ver string) error {
	current := GoupCurrentDir()
	version := goupVersionDir(ver)

	if _, err := os.Stat(version); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Go version %s is not installed. Install it with `goup install`.", ver)

		} else {
			return err
		}
	}

	// ignore error, similar to rm -f
	os.Remove(current)

	return os.Symlink(version, current)
}

func install(release entity.Release) (err error) {
	svc := service.NewGoReleaseService(GetGoHost())

	version := release.Version
	targetDir := goupVersionDir(version)

	if checkInstalled(targetDir) {
		logger.Printf("%s: already installed in %v", version, targetDir)
		return nil
	}

	fg, err := release.ArchiveFile()
	if err != nil {
		return
	}

	fileUrl := fg.Url(GetGoHost())
	code, contentLength, err := svc.CheckArchiveFileExists(fileUrl)

	if err != nil {
		return err
	}
	if code == http.StatusNotFound {
		return fmt.Errorf("no binary release of %v for %v/%v at %v", version, getOS(), getGoArch(), fileUrl)
	}
	if code != http.StatusOK {
		return fmt.Errorf("server returned %v checking size of %v", http.StatusText(code), fileUrl)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	archiveFile := filepath.Join(targetDir, fg.Filename)
	if fi, err := os.Stat(archiveFile); err != nil || fi.Size() != contentLength {
		if err != nil && !os.IsNotExist(err) {
			// Something weird. Don't try to download.
			return err
		}
		if err := svc.DownloadFile(archiveFile, fileUrl); err != nil {
			return fmt.Errorf("error downloading %v: %v", fileUrl, err)
		}
		fi, err = os.Stat(archiveFile)
		if err != nil {
			return err
		}
		if fi.Size() != contentLength {
			return fmt.Errorf("downloaded file %s size %v doesn't match server size %v", archiveFile, fi.Size(), contentLength)
		}
	}

	wantSHA := fg.Sha256

	if err := verifySHA256(archiveFile, strings.TrimSpace(wantSHA)); err != nil {
		return fmt.Errorf("error verifying SHA256 of %v: %v", archiveFile, err)
	}
	logger.Printf("Unpacking %v ...", archiveFile)
	if err := unpackArchive(targetDir, archiveFile); err != nil {
		return fmt.Errorf("extracting archive %v: %v", archiveFile, err)
	}

	if err := setInstalled(targetDir); err != nil {
		return err
	}
	logger.Printf("Success: %s installed in %v", version, targetDir)
	return nil
}

func installTip(clNumber string) error {
	root := goupVersionDir("gotip")

	git := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = root
		return cmd.Run()
	}
	gitOutput := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		return cmd.Output()
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if err := os.MkdirAll(root, 0755); err != nil {
			return fmt.Errorf("failed to create repository: %v", err)
		}

		if err := git("clone", "--depth=1", GetGoSourceGitURL(), root); err != nil {
			return fmt.Errorf("failed to clone git repository: %v", err)
		}

		gsuURL := os.Getenv("GOUP_GO_SOURCE_GIT_URL")
		if gsuURL == "" {
			gsuURL = goSourceUpsteamGitURL
		}

		if err := git("remote", "add", "upstream", gsuURL); err != nil {
			return fmt.Errorf("failed to add upstream git repository: %v", err)
		}
	}

	if clNumber != "" {
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("This will download and execute code from go.dev/cl/%s, continue", clNumber),
			IsConfirm: true,
		}

		if _, err := prompt.Run(); err != nil {
			return fmt.Errorf("interrupted")
		}

		// CL is for googlesource, ls-remote against upstream
		// ls-remote outputs a number of lines like:
		// 2621ba2c60d05ec0b9ef37cd71e45047b004cead	refs/changes/37/227037/1
		// 51f2af2be0878e1541d2769bd9d977a7e99db9ab	refs/changes/37/227037/2
		// af1f3b008281c61c54a5d203ffb69334b7af007c	refs/changes/37/227037/3
		// 6a10ebae05ce4b01cb93b73c47bef67c0f5c5f2a	refs/changes/37/227037/meta
		refs, err := gitOutput("ls-remote", "upstream")
		if err != nil {
			return fmt.Errorf("failed to list remotes: %v", err)
		}
		r := regexp.MustCompile(`refs/changes/\d\d/` + clNumber + `/(\d+)`)
		match := r.FindAllStringSubmatch(string(refs), -1)
		if match == nil {
			return fmt.Errorf("CL %v not found", clNumber)
		}
		var ref string
		var patchSet int
		for _, m := range match {
			ps, _ := strconv.Atoi(m[1])
			if ps > patchSet {
				patchSet = ps
				ref = m[0]
			}
		}
		logger.Printf("Fetching CL %v, Patch Set %v...", clNumber, patchSet)
		if err := git("fetch", "upstream", ref); err != nil {
			return fmt.Errorf("failed to fetch %s: %v", ref, err)
		}
	} else {
		logger.Printf("Updating the go development tree...")
		if err := git("fetch", "origin", "master"); err != nil {
			return fmt.Errorf("failed to fetch git repository updates: %v", err)
		}
	}

	// Use checkout and a detached HEAD, because it will refuse to overwrite
	// local changes, and warn if commits are being left behind, but will not
	// mind if master is force-pushed upstream.
	if err := git("-c", "advice.detachedHead=false", "checkout", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("failed to checkout git repository: %v", err)
	}
	// It shouldn't be the case, but in practice sometimes binary artifacts
	// generated by earlier Go versions interfere with the build.
	//
	// Ask the user what to do about them if they are not gitignored. They might
	// be artifacts that used to be ignored in previous versions, or precious
	// uncommitted source files.
	if err := git("clean", "-i", "-d"); err != nil {
		return fmt.Errorf("failed to cleanup git repository: %v", err)
	}
	// Wipe away probably boring ignored files without bothering the user.
	if err := git("clean", "-q", "-f", "-d", "-X"); err != nil {
		return fmt.Errorf("failed to cleanup git repository: %v", err)
	}

	cmd := exec.Command(filepath.Join(root, "src", makeScript()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Join(root, "src")
	if runtime.GOOS == "windows" {
		// Workaround make.bat not autodetecting GOROOT_BOOTSTRAP. Issue 28641.
		goroot, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			return fmt.Errorf("failed to detect an existing go installation for bootstrap: %v", err)
		}
		cmd.Env = append(os.Environ(), "GOROOT_BOOTSTRAP="+strings.TrimSpace(string(goroot)))
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build go: %v", err)
	}

	return nil
}

// unpackArchive unpacks the provided archive zip or tar.gz file to targetDir,
// removing the "go/" prefix from file entries.
func unpackArchive(targetDir, archiveFile string) error {
	switch {
	case strings.HasSuffix(archiveFile, ".zip"):
		return unpackZip(targetDir, archiveFile)
	case strings.HasSuffix(archiveFile, ".tar.gz"):
		return unpackTarGz(targetDir, archiveFile)
	default:
		return errors.New("unsupported archive file")
	}
}

// unpackTarGz is the tar.gz implementation of unpackArchive.
func unpackTarGz(targetDir, archiveFile string) error {
	r, err := os.Open(archiveFile)
	if err != nil {
		return err
	}
	defer r.Close()
	madeDir := map[string]bool{}
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		f, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !validRelPath(f.Name) {
			return fmt.Errorf("tar file contained invalid name %q", f.Name)
		}
		rel := filepath.FromSlash(strings.TrimPrefix(f.Name, "go/"))
		abs := filepath.Join(targetDir, rel)

		fi := f.FileInfo()
		mode := fi.Mode()
		switch {
		case mode.IsRegular():
			// Make the directory. This is redundant because it should
			// already be made by a directory entry in the tar
			// beforehand. Thus, don't check for errors; the next
			// write will fail with the same error.
			dir := filepath.Dir(abs)
			if !madeDir[dir] {
				if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
					return err
				}
				madeDir[dir] = true
			}
			wf, err := os.OpenFile(abs, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm())
			if err != nil {
				return err
			}
			n, err := io.Copy(wf, tr)
			if closeErr := wf.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
			if err != nil {
				return fmt.Errorf("error writing to %s: %v", abs, err)
			}
			if n != f.Size {
				return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, abs, f.Size)
			}
			if !f.ModTime.IsZero() {
				if err := os.Chtimes(abs, f.ModTime, f.ModTime); err != nil {
					// benign error. Gerrit doesn't even set the
					// modtime in these, and we don't end up relying
					// on it anywhere (the gomote push command relies
					// on digests only), so this is a little pointless
					// for now.
					logger.Printf("error changing modtime: %v", err)
				}
			}
		case mode.IsDir():
			if err := os.MkdirAll(abs, 0755); err != nil {
				return err
			}
			madeDir[abs] = true
		default:
			return fmt.Errorf("tar file entry %s contained unsupported file type %v", f.Name, mode)
		}
	}
	return nil
}

// unpackZip is the zip implementation of unpackArchive.
func unpackZip(targetDir, archiveFile string) error {
	zr, err := zip.OpenReader(archiveFile)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "go/")

		outpath := filepath.Join(targetDir, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outpath, 0755); err != nil {
				return err
			}
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		// File
		if err := os.MkdirAll(filepath.Dir(outpath), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(outpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// verifySHA256 reports whether the named file has contents with
// SHA-256 of the given wantHex value.
func verifySHA256(file, wantHex string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != wantHex {
		return fmt.Errorf("%s corrupt? does not have expected SHA-256 of %v", file, wantHex)
	}
	return nil
}

func makeScript() string {
	switch runtime.GOOS {
	case "plan9":
		return "make.rc"
	case "windows":
		return "make.bat"
	default:
		return "make.bash"
	}
}

// unpackedOkay is a sentinel zero-byte file to indicate that the Go
// version was downloaded and unpacked successfully.
const unpackedOkay = ".unpacked-success"

func validRelPath(p string) bool {
	if p == "" || strings.Contains(p, `\`) || strings.HasPrefix(p, "/") || strings.Contains(p, "../") {
		return false
	}
	return true
}
