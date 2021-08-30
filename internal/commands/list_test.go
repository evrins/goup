package commands

import (
	"github.com/blang/semver/v4"
	"log"
	"testing"
)

func Test_getVersionList(t *testing.T) {
	rs, err := getReleaseList("")
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(rs)
}
