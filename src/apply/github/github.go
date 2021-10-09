package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	googlegithub "github.com/google/go-github/v39/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

type Config struct {
	AccessToken string
}

type T struct {
	cfg    Config
	client *googlegithub.Client
}

func New(cfg Config) *T {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.AccessToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	client := googlegithub.NewClient(tc)

	return &T{
		cfg:    cfg,
		client: client,
	}
}

type Repository struct {
	cfg  Config
	repo *git.Repository
}

type AddOptions struct {
	All bool
}

func (repo *Repository) Add(options AddOptions) error {
	worktree, err := repo.repo.Worktree()
	if err != nil {
		return errors.WithStack(err)
	}

	err = worktree.AddWithOptions(&git.AddOptions{All: options.All})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (repo *Repository) FilesAffected() (files []string, err error) {
	worktree, err := repo.repo.Worktree()
	if err != nil {
		return files, errors.WithStack(err)
	}

	status, err := worktree.Status()
	if err != nil {
		return files, errors.WithStack(err)
	}

	for fileName := range status {
		files = append(files, fileName)
	}

	return files, nil
}

type CommitOptions struct {
	All bool
}

func (repo *Repository) Commit(message string, options CommitOptions) error {
	worktree, err := repo.repo.Worktree()
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = worktree.Commit(message, &git.CommitOptions{All: options.All})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (repo *Repository) Push() error {
	err := repo.repo.Push(&git.PushOptions{Auth: &http.BasicAuth{
		Username: "_",
		Password: repo.cfg.AccessToken,
	}})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type CheckoutOptions struct {
	Branch string
	Create bool
	Force  bool
}

func (repo *Repository) Checkout(options CheckoutOptions) error {
	worktree, err := repo.repo.Worktree()
	if err != nil {
		return errors.WithStack(err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(options.Branch),
		Create: options.Create,
		Force:  options.Force,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

type CloneOptions struct {
	RepoURL string
	Folder  string
}

func (github *T) Clone(options CloneOptions) (out Repository, err error) {
	repo, err := git.PlainClone(options.Folder, false, &git.CloneOptions{
		URL:               options.RepoURL,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		Auth: &http.BasicAuth{
			Username: "_",
			Password: github.cfg.AccessToken,
		},
	})
	if err != nil {
		return out, errors.WithStack(err)
	}

	out = Repository{
		cfg:  github.cfg,
		repo: repo,
	}

	return out, nil
}

type PullRequestOptions struct {
	RepoURL     string
	Title       string
	FromBranch  string
	ToBranch    string
	Description string
}

func (github *T) PullRequest(options PullRequestOptions) (*googlegithub.PullRequest, error) {
	repoInfo := parseRepoURL(options.RepoURL)

	pullRequest, _, err := github.client.PullRequests.Create(context.Background(), repoInfo.Owner, repoInfo.Name, &googlegithub.NewPullRequest{
		Title:               googlegithub.String(options.Title),
		Head:                googlegithub.String(options.FromBranch),
		Base:                googlegithub.String(options.ToBranch),
		Body:                googlegithub.String(options.Description),
		MaintainerCanModify: googlegithub.Bool(true),
	})
	if err != nil {
		return pullRequest, errors.WithStack(err)
	}

	return pullRequest, nil
}

type RepoInfo struct {
	Owner string
	Name  string
}

func filterOutEmptyStrings(ss []string) []string {
	out := make([]string, 0)

	for _, s := range ss {
		if s == "" {
			continue
		}

		out = append(out, s)
	}

	return out
}

func parseRepoURL(repoURL string) RepoInfo {
	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		panic(errors.Wrapf(err, "invalid url: %s", repoURL))
	}

	parts := filterOutEmptyStrings(strings.Split(parsedURL.Path, "/"))
	if len(parts) < 2 {
		panic(fmt.Sprintf("invalid url: %s", repoURL))
	}

	return RepoInfo{
		Owner: parts[0],
		Name:  parts[1],
	}
}
