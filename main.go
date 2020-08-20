package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

type PluginRelease struct {
	CreatedAt time.Time
}

type Plugin struct {
	Type          string
	Name          string
	LatestRelease string
	Versions      map[string]*PluginRelease
}

type Plugins struct {
	LastUpdate time.Time
	Plugins    []*Plugin
}

const DestDir = "./plugin-index/api/v1/"

func writePlugins(path string, plugins *Plugins) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plugins); err != nil {
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

func getPluginReleases(owner, repo string) (map[string]*PluginRelease, error) {
	releases, _, err := ghClient.Repositories.ListReleases(context.Background(), owner, repo, &github.ListOptions{PerPage: 100})
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
	for i, p := range pList {
		tp, err := transformPlugin(p)
		checkError(err)
		transformedPlugins[i] = tp
	}

	fmt.Printf("creating %s\n", DestDir)
	checkError(os.RemoveAll("./plugin-index"))
	checkError(os.MkdirAll(DestDir, 0755))

	plugins := &Plugins{
		LastUpdate: time.Now(),
		Plugins:    transformedPlugins,
	}

	plugJson := path.Join(DestDir, "plugins.json")
	fmt.Printf("creating %s\n", plugJson)
	checkError(writePlugins(plugJson, plugins))
}
