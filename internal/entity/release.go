package entity

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
)

type Kind string

var Archive Kind = "archive"
var Installer Kind = "installer"
var Source Kind = "source"

type Release struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []File `json:"files"`
}

func (r Release) ArchiveFile() (file File, err error) {
	goos := getOS()
	arch := runtime.GOARCH

	if goos == "linux" && arch == "arm" {
		arch = "armv6l"
	}

	for _, f := range r.Files {
		if f.Arch == arch && f.Os == goos && f.Kind == Archive {
			file = f
			return
		}
	}
	err = errors.New(fmt.Sprintf("target os %s arch %s archive not found", goos, arch))
	return
}

func getOS() string {
	return runtime.GOOS
}

type File struct {
	Filename string `json:"filename"`
	Os       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	Sha256   string `json:"sha256"`
	Size     int    `json:"size"`
	Kind     Kind   `json:"kind"`
}

func (f File) Url(goHost string) string {
	return fmt.Sprintf("https://%s/dl/%s", goHost, f.Filename)
}

type ReleaseList []Release

func (r ReleaseList) Len() int {
	return len(r)
}

func (r ReleaseList) Less(i, j int) bool {
	return r[i].Version < r[j].Version
}

func (r ReleaseList) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r ReleaseList) VersionList() (rs []string) {
	for _, v := range r {
		rs = append(rs, v.Version)
	}
	sort.Strings(rs)
	return
}
