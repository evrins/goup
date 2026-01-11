package commands

import (
	"fmt"
	"github.com/owenthereal/goup/internal/entity"
	"github.com/owenthereal/goup/internal/global"

	"regexp"
	"sort"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

func searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search [REGEXP]",
		Short: `Search Go versions to install`,
		Long: `Search available Go versions matching a regexp filter for installation. If no filter is provided,
list all available versions.`,
		Example: `
  goup search
  goup search 1.15
`,
		RunE: runSearch,
	}
}

func runSearch(cmd *cobra.Command, args []string) error {
	var regexp string
	if len(args) > 0 {
		regexp = args[0]
	}

	vers, err := listGoVersions(regexp)
	if err != nil {
		return err
	}

	for _, ver := range vers {
		fmt.Println(ver)
	}

	return nil
}

func listGoVersions(re string) ([]string, error) {
	if re == "" {
		re = ".+"
	} else {
		re = fmt.Sprintf(`.*%s.*`, re)

	}

	cmd := exec.Command("git", "ls-remote", "--sort=version:refname", "--tags", GetGoSourceGitURL())
	refs, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var versionList []string
	for _, v := range rl.VersionList() {
		if r.MatchString(v) {
			versionList = append(versionList, v)
		}
	}
	if len(versionList) == 0 {
		return nil, fmt.Errorf("No Go version found")
	}

	return versionList, nil
}

func getVersionListWithFilter(filter string) (rs entity.ReleaseList, err error) {
	rl, err := getReleaseList("all")
	filter = strings.TrimPrefix(filter, "go")
	filter = "go" + filter
	for _, v := range rl {
		if strings.HasPrefix(v.Version, filter) {
			rs = append(rs, v)
		}
	}
	return
}

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
