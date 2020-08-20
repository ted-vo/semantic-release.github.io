package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

type PluginRelease struct {
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
	data, err := json.Marshal(plugins)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0755)
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
	return &Plugin{
		Type:          strings.ToLower(p.Type),
		Name:          strings.ToLower(p.Name),
		LatestRelease: release.GetTagName(),
		Versions:      nil,
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
