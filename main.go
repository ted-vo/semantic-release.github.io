package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v37/github"
	"golang.org/x/oauth2"
)

var osArchRe = regexp.MustCompile(`(?i)(aix|android|darwin|dragonfly|freebsd|hurd|illumos|js|linux|nacl|netbsd|openbsd|plan9|solaris|windows|zos)(_|-)(386|amd64|amd64p32|arm|armbe|arm64|arm64be|ppc64|ppc64le|mips|mipsle|mips64|mips64le|mips64p32|mips64p32le|ppc|riscv|riscv64|s390|s390x|sparc|sparc64|wasm)(\.exe)?$`)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

type PluginAsset struct {
	FileName string
	URL      string
	OS       string
	Arch     string
	Checksum string
}

type PluginRelease struct {
	CreatedAt time.Time
	Assets    []*PluginAsset
}

type Plugin struct {
	Type          string
	Name          string
	LatestRelease string
	Versions      map[string]*PluginRelease
}

func (p *Plugin) CheckLatestRelease() error {
	pr := p.Versions[p.LatestRelease]
	if pr == nil {
		return errors.New("latest release not found")
	}
	if len(pr.Assets) == 0 {
		return errors.New("latest release assets not found")
	}
	return nil
}

type Plugins struct {
	Plugins []string
}

const DestDir = "./plugin-index/api/v1/"

func writeJSON(path string, data interface{}) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return err
	}
	return nil
}

type pluginListPlugin struct {
	Type string
	Name string
	Repo string
}

func readPluginList() ([]*pluginListPlugin, error) {
	file, err := os.OpenFile("./plugin-list.json", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var data []*pluginListPlugin
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

var ghClient *github.Client

func init() {
	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		fmt.Println("GITHUB_TOKEN missing")
		os.Exit(1)
	}
	ghClient = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)))
}

func fetchChecksumFile(url string) map[string]string {
	fmt.Printf("fetching checksums %s\n", url)
	ret := make(map[string]string)
	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	checksums, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		fmt.Println(err)
		return nil
	}
	for _, l := range strings.Split(string(checksums), "\n") {
		sl := strings.Split(l, " ")
		if len(sl) < 3 {
			continue
		}
		ret[strings.ToLower(sl[2])] = sl[0]
	}
	return ret
}

func getPluginAssets(gha []*github.ReleaseAsset) []*PluginAsset {
	ret := make([]*PluginAsset, 0)
	var checksumMap map[string]string = nil
	for _, asset := range gha {
		fn := asset.GetName()
		if checksumMap == nil && asset.GetSize() <= 4096 && strings.Contains(strings.ToLower(fn), "checksums.txt") {
			checksumMap = fetchChecksumFile(asset.GetBrowserDownloadURL())
			continue
		}
		ret = append(ret, &PluginAsset{
			FileName: fn,
			URL:      asset.GetBrowserDownloadURL(),
		})
	}
	for i, pa := range ret {
		if checksumMap != nil {
			ret[i].Checksum = checksumMap[strings.ToLower(ret[i].FileName)]
		}
		osArch := osArchRe.FindAllStringSubmatch(pa.FileName, -1)
		if len(osArch) < 1 || len(osArch[0]) < 4 {
			continue
		}
		ret[i].OS = strings.ToLower(osArch[0][1])
		ret[i].Arch = strings.ToLower(osArch[0][3])
	}
	return ret
}

func getAllGitHubReleases(owner, repo string) ([]*github.RepositoryRelease, error) {
	ret := make([]*github.RepositoryRelease, 0)
	opts := &github.ListOptions{Page: 1, PerPage: 100}
	for {
		releases, resp, err := ghClient.Repositories.ListReleases(context.Background(), owner, repo, opts)
		if err != nil {
			return nil, err
		}
		ret = append(ret, releases...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return ret, nil
}

func getPluginReleases(owner, repo string) (map[string]*PluginRelease, error) {
	releases, err := getAllGitHubReleases(owner, repo)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]*PluginRelease)
	for _, r := range releases {
		if r.GetDraft() {
			continue
		}
		v, err := semver.NewVersion(r.GetTagName())
		if err != nil {
			continue
		}
		vStr := v.String()
		ret[vStr] = &PluginRelease{
			CreatedAt: r.GetCreatedAt().Time,
			Assets:    getPluginAssets(r.Assets),
		}
	}

	return ret, nil
}

func transformPlugin(p *pluginListPlugin) (*Plugin, error) {
	fmt.Printf("transforming %s (%s-%s)\n", p.Repo, p.Type, p.Name)
	split := strings.SplitN(p.Repo, "/", 2)
	if len(split) < 2 {
		return nil, errors.New("invalid repo")
	}
	owner, repo := split[0], split[1]

	release, _, err := ghClient.Repositories.GetLatestRelease(context.Background(), owner, repo)
	if err != nil {
		return nil, err
	}
	lrVersion, err := semver.NewVersion(release.GetTagName())
	if err != nil {
		return nil, err
	}

	vers, err := getPluginReleases(owner, repo)
	if err != nil {
		return nil, err
	}

	return &Plugin{
		Type:          strings.ToLower(p.Type),
		Name:          strings.ToLower(p.Name),
		LatestRelease: lrVersion.String(),
		Versions:      vers,
	}, nil
}

func main() {
	fmt.Println("reading plugin list...")
	pList, err := readPluginList()
	checkError(err)

	fmt.Println("transforming plugins...")
	transformedPlugins := make([]*Plugin, len(pList))
	pluginNames := make([]string, len(pList))
	for i, p := range pList {
		tp, err := transformPlugin(p)
		checkError(err)
		checkError(tp.CheckLatestRelease())
		pluginNames[i] = tp.Type + "-" + tp.Name
		transformedPlugins[i] = tp
	}

	fmt.Printf("creating %s\n", DestDir)
	checkError(os.RemoveAll("./plugin-index"))
	checkError(os.MkdirAll(DestDir, 0755))

	plugDir := path.Join(DestDir, "plugins")
	fmt.Printf("creating %s\n", plugDir)
	checkError(os.MkdirAll(plugDir, 0755))

	for _, p := range transformedPlugins {
		plugPath := path.Join(plugDir, p.Type+"-"+p.Name+".json")
		fmt.Printf("creating %s\n", plugPath)
		checkError(writeJSON(plugPath, p))
	}

	plugins := &Plugins{
		Plugins: pluginNames,
	}

	plugJson := path.Join(DestDir, "plugins.json")
	fmt.Printf("creating %s\n", plugJson)
	checkError(writeJSON(plugJson, plugins))
}
