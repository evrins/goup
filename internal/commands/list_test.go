package commands

import (
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
