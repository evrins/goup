package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/owenthereal/goup/internal/entity"
)

type GoReleaseService struct {
	goHost string
	client *resty.Client
}

func NewGoReleaseService(goHost string) *GoReleaseService {
	client := resty.New()

	version := runtime.Version()
	if strings.ContainsAny(version, "devel") {
		version = "devel"
	}
	client.SetHeader("User-Agent", "goup/"+version)

	client.SetRetryCount(10)

	return &GoReleaseService{
		goHost: goHost,
		client: client,
	}
}

// GetReleaseList include: "all" or ""
func (svc *GoReleaseService) GetReleaseList(include string) (rl entity.ReleaseList, err error) {
	link := fmt.Sprintf("https://%s/dl/", svc.goHost)

	_, err = svc.client.R().
		SetQueryParam("mode", "json").
		SetQueryParam("include", include).
		SetResult(&rl).
		Get(link)
	if err != nil {
		return
	}

	sort.Sort(rl)
	return
}

func (svc *GoReleaseService) GetReleaseWithFilter(filter string) (res entity.ReleaseList, err error) {
	rl, err := svc.GetReleaseList("all")
	if err != nil {
		return
	}

	filter = strings.TrimPrefix(filter, "go")
	filter = fmt.Sprintf("go%s", filter)

	for _, r := range rl {
		if strings.HasPrefix(r.Version, filter) {
			res = append(res, r)
		}
	}

	return
}

func (svc *GoReleaseService) GetLatestRelease() (r entity.Release, err error) {
	rl, err := svc.GetReleaseList("")
	if err != nil {
		return
	}
	r = rl[len(rl)-1]
	return
}

func (svc *GoReleaseService) CheckArchiveFileExists(archiveUrl string) (code int, contentLength int64, err error) {
	response, err := svc.client.R().
		Head(archiveUrl)
	if err != nil {
		return
	}

	code = response.StatusCode()
	contentLength = response.RawResponse.ContentLength
	return
}

func (svc *GoReleaseService) DownloadFile(destFile, fileUrl string) (err error) {
	f, err := os.Create(destFile)
	defer func() {
		err = f.Close()
	}()

	if err != nil {
		return
	}

	resp, err := svc.client.R().SetDoNotParseResponse(true).Get(fileUrl)
	if err != nil {
		return
	}

	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	var body = resp.RawBody()
	var contentLength = resp.RawResponse.ContentLength
	defer body.Close()
	pw := NewProgressWriter(f, contentLength)
	n, err := io.Copy(pw, body)
	if err != nil {
		return
	}
	if contentLength != -1 && contentLength != n {
		return fmt.Errorf("copied %v bytes; expected %v", n, contentLength)
	}
	pw.Update()
	return
}
