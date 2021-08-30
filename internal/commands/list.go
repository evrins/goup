package commands

import (
	"fmt"
	"github.com/owenthereal/goup/internal/entity"
	"github.com/owenthereal/goup/internal/global"
	"sort"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls-ver [regexp]",
		Short: `List Go versions to install`,
		Long: `List available Go versions matching a regexp filter for installation. If no filter is provided,
list all available versions.`,
		Example: `
  goup ls-ver
  goup ls-ver 1.15
`,
		RunE: runList,
	}
}

func runList(cmd *cobra.Command, args []string) (err error) {
	var filter string
	if len(args) > 0 {
		filter = args[0]
	}

	filter = strings.TrimSpace(filter)
	var rl entity.ReleaseList
	if filter == "" {
		rl, err = getReleaseList("all")
	} else {
		rl, err = getVersionListWithFilter(filter)
	}

	if err != nil {
		return err
	}

	for _, v := range rl {
		fmt.Println(strings.TrimPrefix(v.Version, "go"))
	}

	return nil
}

func getVersionListWithFilter(filter string) (rs entity.ReleaseList, err error) {
	rl, err := getReleaseList("all")
	if err != nil {
		return
	}
	for _, v := range rl {
		if strings.Contains(v.Version, filter) {
			rs = append(rs, v)
		}
	}
	return
}

// include = "all" to get all version
// include = "" to get only recent version
func getReleaseList(include string) (rl entity.ReleaseList, err error) {
	// the tailing slash is the key to call api
	link := fmt.Sprintf("https://%s/dl/", global.GoHost)
	client := resty.New()

	_, err = client.R().
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
