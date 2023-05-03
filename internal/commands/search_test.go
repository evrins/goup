package commands

import (
	"github.com/owenthereal/goup/internal/global"
	"log"
	"testing"
)

func init() {
	global.GoHost = "golang.google.cn"
}

func Test_getReleaseList(t *testing.T) {
	rs, err := getReleaseList("all")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(rs)
}
