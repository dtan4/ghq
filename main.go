package main

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/motemen/ghq/pocket"
	"github.com/motemen/ghq/utils"
)

func main() {
	app := cli.NewApp()
	app.Name = "ghq"
	app.Usage = "Manage GitHub repository clones"
	app.Version = "0.1.0"
	app.Author = "motemen"
	app.Email = "motemen@gmail.com"
	app.Commands = []cli.Command{
		{
			Name:   "get",
			Usage:  "Clone/sync with a remote repository",
			Action: CommandGet,
		},
		{
			Name:   "list",
			Usage:  "List local repositories",
			Action: CommandList,
			Flags: []cli.Flag{
				cli.BoolFlag{"exact, e", "Perform an exact match"},
			},
		},
		{
			Name: "pocket",
			Action: func(c *cli.Context) {
				accessToken, err := GitConfig("ghq.pocket.token")
				mustBeOkay(err)

				if accessToken == "" {
					receiverURL, ch, err := pocket.StartAccessTokenReceiver()
					mustBeOkay(err)

					authRequest, err := pocket.ObtainRequestToken(receiverURL)
					mustBeOkay(err)

					url := pocket.GenerateAuthorizationURL(authRequest.Code, receiverURL)
					utils.Log("open", url)

					<-ch

					authorized, err := pocket.ObtainAccessToken(authRequest.Code)
					mustBeOkay(err)

					accessToken = authorized.AccessToken
					Git("config", "ghq.pocket.token", authorized.AccessToken)
				}

				res, err := pocket.RetrieveGitHubEntries(accessToken)
				mustBeOkay(err)

				for _, item := range res.List {
					u, err := ParseGitHubURL(item.ResolvedURL)
					if err != nil {
						utils.Log("error", err.Error())
						continue
					} else if u.User == "blog" {
						utils.Log("skip", fmt.Sprintf("%s: is not a repository", u))
						continue
					} else if u.Extra != "" {
						utils.Log("skip", fmt.Sprintf("%s: is not project home", u))
						continue
					}

					GetGitHubRepository(u)
				}
			},
		},
	}

	app.Run(os.Args)
}

func mustBeOkay(err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		utils.Log("error", fmt.Sprintf("at %s line %d: %s", file, line, err))
		os.Exit(1)
	}
}

func CommandGet(c *cli.Context) {
	argUrl := c.Args().Get(0)

	if argUrl == "" {
		cli.ShowCommandHelp(c, "get")
		os.Exit(1)
	}

	u, err := ParseGitHubURL(argUrl)
	mustBeOkay(err)

	GetGitHubRepository(u)
}

func GetGitHubRepository(u *GitHubURL) {
	path := pathForRepository(u)

	newPath := false

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			newPath = true
			err = nil
		}
		mustBeOkay(err)
	}

	if newPath {
		utils.Log("clone", fmt.Sprintf("%s/%s -> %s", u.User, u.Repo, path))

		dir, _ := filepath.Split(path)
		mustBeOkay(os.MkdirAll(dir, 0755))
		Git("clone", u.String(), path)
	} else {
		utils.Log("update", path)

		mustBeOkay(os.Chdir(path))
		Git("remote", "update")
	}
}

func CommandList(c *cli.Context) {
	query := c.Args().First()
	exact := c.Bool("exact")

	var filterFn func(string, string, string) bool
	if query == "" {
		filterFn = func(_, _, _ string) bool { return true }
	} else if exact {
		filterFn = func(relPath, user, repo string) bool { return relPath == query || repo == query }
	} else {
		filterFn = func(relPath, user, repo string) bool { return strings.Contains(relPath, query) }
	}

	walkLocalRepositories(func(relPath, user, repo string) {
		if filterFn(relPath, user, repo) == false {
			return
		}

		fmt.Println(relPath)
	})
}

func walkLocalRepositories(callback func(string, string, string)) {
	root := reposRoot()
	filepath.Walk(root, func(path string, fileInfo os.FileInfo, err error) error {
		rel, err := filepath.Rel(root, path)
		mustBeOkay(err)

		user, repo := filepath.Split(rel)
		if user == "" || repo == "" {
			return nil
		}

		callback(rel, user, repo)

		return filepath.SkipDir
	})

	return
}

func reposRoot() string {
	reposRoot, err := GitConfig("ghq.root")
	mustBeOkay(err)

	if reposRoot == "" {
		usr, err := user.Current()
		mustBeOkay(err)

		reposRoot = path.Join(usr.HomeDir, ".ghq", "repos")
	}

	return reposRoot
}

func pathForRepository(u *GitHubURL) string {
	return path.Join(reposRoot(), "@"+u.User, u.Repo)
}
