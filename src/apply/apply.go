package apply

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"apply_codemod/src/apply/github"
	"apply_codemod/src/codemod"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
	"github.com/tcnksm/go-input"
)

const tempFolder = "./codemod_tmp"

type Codemod struct {
	Description string
	Transform   interface{}
}

// Contains command line arguments that the user can provide.
type CliArgs struct {
	// Token used to clone and make pull requests on github.
	GithubToken string `long:"github_token" description:"github access token" required:"true"`
	// Github user that owns the target repositories.
	GithubUser *string `long:"github_user" description:"github user that owns the target repositories"`
	// Github organization that owns the target repositories.
	GithubOrg *string `long:"github_org" description:"github organization that owns the target repositories"`
	// Github search api input. Will be used to find repositories
	// with specific names.
	RepoNameMatches *string `long:"repo_name_matches" description:"regex used to match repositories. codemods will be applied to any repository that matches the regex"`
	// Github search api input. Will be used to find repositories
	// that contain the specified contents.
	RepoContains *string `long:"repo_contains" description:"contents to look for in repositories"`
	// If the user wants to apply codemods to a directory in
	// their machine, they can inform it using the --local_dir flag.
	LocalDirectory *string `long:"local_dir" description:"directory on your machine that codemods should be applied to"`
	// List of repositories to apply codemods to.
	//
	// Should be in the format repo_url:branch.
	//
	// Should be possible to clone, create branches and
	// create pull requests for each repository
	// using the github token provided.
	Repositories map[string]string `long:"repos" description:"list of repositories to apply codemod to. should be a list of repository_url:branch"`
	//
	//
	// Should be in the format regex_to_match:new_value.
	Replacements map[string]string `long:"replace" description:"replaces whatever matches the regex on left to whatever is on the right"`
}

var ErrArgumentIsRequired = errors.New("argument is required")

// Parses command line arguments and returns a struct
// containing the arguments we expect.
//
// `args` is os.Args[1:].
//
// We take a list of args to make testing easier.
func getCliArgs(args []string) (CliArgs, error) {
	var cliArgs CliArgs

	// We use the default parser options and
	// also ignore unknown arguments.
	_, err := flags.NewParser(&cliArgs, flags.Default|flags.IgnoreUnknown|flags.HelpFlag).ParseArgs(args)
	if err != nil {
		return cliArgs, errors.WithStack(err)
	}

	// TODO: this is bad
	if cliArgs.LocalDirectory == nil && len(cliArgs.Repositories) == 0 && cliArgs.GithubUser == nil && cliArgs.GithubOrg == nil {
		return cliArgs, errors.Wrap(
			ErrArgumentIsRequired,
			"If a list of repositories is not informed, a github user or organization must be",
		)
	}

	return cliArgs, nil
}

// Responsible for applying codemods to local directories
// and to remove repositories.
type Applier struct {
	// User provided command line arguments.
	args CliArgs
	// List of codemods to apply.
	codemods []Codemod
	// Client used to interact with the Github api.
	githubClient *github.T
	// Library used to get input from the user.
	//
	// It helps us ask questions like
	// >> Do you want to proceed? [yes/no]
	//
	// and wait for the user to type yes or no, for example.
	ui *input.UI
}

func New() (out Applier, err error) {
	// Skip the first value in os.Args because it
	// is the directory from where the binary
	// was executed and we don't need it.
	args, err := getCliArgs(os.Args[1:])
	if err != nil {
		return out, errors.WithStack(err)
	}

	githubClient := github.New(github.Config{
		AccessToken: args.GithubToken,
	})

	ui := &input.UI{
		Writer: os.Stdout,
		Reader: os.Stdin,
	}

	out = Applier{args: args, githubClient: githubClient, ui: ui}

	return out, nil
}

// Returns true when codemods should be applied to a local directory.
func (applier *Applier) ShouldApplyLocally() bool {
	return applier.args.LocalDirectory != nil
}

// If the user provided a list of repositories in the command line,
// returns the user provided list.
//
// Otherwise, gets repositories from github using the username or organization
// provided by the user in the command line.
func (applier *Applier) getRepositories(ctx context.Context) (out []Repository, err error) {
	if len(applier.args.Repositories) > 0 {
		for repoURL, branch := range applier.args.Repositories {
			branch := branch
			out = append(out, Repository{URL: repoURL, Branch: &branch})
		}
	} else {
		var query string

		if applier.args.GithubOrg != nil {
			query = fmt.Sprintf("org:%s", *applier.args.GithubOrg)
		} else if applier.args.GithubUser != nil {
			query = fmt.Sprintf("user:%s", *applier.args.GithubUser)
		}

		if applier.args.RepoContains != nil {
			query = fmt.Sprintf("%s %s", *applier.args.RepoContains, query)

			log.Printf("code searching. query=%s\n", query)

			searchResult, err := applier.githubClient.CodeSearch(ctx, query)
			if err != nil {
				return out, errors.WithStack(err)
			}

			for _, result := range searchResult.CodeResults {
				textMatches := make([]TextMatch, 0, len(result.TextMatches))

				for _, textMatch := range result.TextMatches {
					textMatches = append(textMatches, TextMatch{
						Fragment: *textMatch.Fragment,
						Match:    *textMatch.Matches[0].Text,
					})
				}
				out = append(out, Repository{
					URL:         *result.Repository.HTMLURL,
					TextMatches: textMatches,
					// We don't know which branch is the default.
					Branch: nil,
				})
			}
		} else if applier.args.RepoNameMatches != nil {
			query = fmt.Sprintf("%s %s", *applier.args.RepoContains, query)

			log.Printf("repository searching. query=%s\n", query)

			searchResult, err := applier.githubClient.RepositorySearch(ctx, query)
			if err != nil {
				return out, errors.WithStack(err)
			}

			for _, repository := range searchResult.Repositories {
				out = append(out, Repository{
					URL:    *repository.HTMLURL,
					Branch: repository.DefaultBranch,
				})
			}
		}
	}

	return out, nil
}

type Range struct {
	start int
	end   int
}

// A text match is a snippet of the repository contents
// that matched a query sent to the Github code search api.
type TextMatch struct {
	// Snippet that matched the query with some of the contents
	// that are close to it.
	Fragment string
	// Contents that matched because they are in `Fragment`.
	Match string
}

type Repository struct {
	// The repository url, used to git clone.
	URL string
	// The branch to which the codemods should be applied.
	//
	// Codemods are applied to the default branch if the branch is not specified.
	Branch *string
	// List of text matches returned by the Github code search api.
	//
	// Note that the list will be empty if the Github code search api wasn't used.
	TextMatches []TextMatch
}

// Applies codemods.
//
// Codemods will be applied to a directory in the
// machine if the --dir flag is present.
//
// Repositories are cloned to the machine
// and codemods are applied if the --dir flag is not present
// and if there's changed files, a pull request is created
// with the changes.
func Apply(ctx context.Context, codemods []Codemod) error {
	applier, err := New()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := applier.apply(ctx); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (applier *Applier) apply(ctx context.Context) error {
	if applier.ShouldApplyLocally() {
		log.Printf("applying codemods to local directory: %s\n", *applier.args.LocalDirectory)

		applier.applyCodemodsLocally(ctx)
	} else {
		log.Println("applying codemods to remote repositories")

		repositories, err := applier.getRepositories(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		log.Printf("found %d repositories", len(repositories))

		repositoriesThatUserWants := make([]Repository, 0)

		for i, repository := range repositories {
			prompt := fmt.Sprintf(`\
Repository %s has %d matches.

The first match is:

~~~

%s

~~~

Do you want to apply codemods to %s ?
`,
				color.GreenString(repository.URL),
				len(repository.TextMatches),
				strings.ReplaceAll(
					repository.TextMatches[0].Fragment,
					repository.TextMatches[0].Match,
					color.GreenString(repository.TextMatches[0].Match),
				),
				repository.URL,
			)
			answer, err := applier.ui.Select(prompt, []string{"yes", "no", "yes to all"}, &input.Options{
				Required: true,
				Loop:     true,
			})
			if err != nil {
				return errors.WithStack(err)
			}

			if answer == "yes" {
				repositoriesThatUserWants = append(repositoriesThatUserWants, repository)
			} else if answer == "yes to all" {
				// Yes to all means:
				//
				// I want to keep the current repository and every other repository
				// that comes after it.
				repositoriesThatUserWants = append(repositoriesThatUserWants, repositories[i:]...)
				break
			}
		}

		if err := applier.applyCodemodsToRemoteRepositories(ctx, repositoriesThatUserWants); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Clones repositories and applies codemods to them.
//
// Pull requests with the changes are created, if there are
// changed files after codemods have been applied.
func (applier *Applier) applyCodemodsToRemoteRepositories(ctx context.Context, repositories []Repository) error {
	for _, repository := range repositories {
		log.Printf("applying codemods to %s", repository.URL)
		githubClient := github.New(github.Config{
			AccessToken: applier.args.GithubToken,
		})

		if err := os.RemoveAll(tempFolder); err != nil {
			return errors.WithStack(err)
		}

		repo, err := githubClient.Clone(github.CloneOptions{
			RepoURL: repository.URL,
			Folder:  tempFolder,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		// If it is not a user specified repository
		// we won't know which branch to apply codemods to.
		//
		// Because of that, we try to apply them to the default branch.
		if repository.Branch == nil {
			branch, err := repo.DefaultBranch()
			if err != nil {
				return errors.WithStack(err)
			}

			repository.Branch = &branch
		}

		err = repo.Checkout(github.CheckoutOptions{
			Branch: *repository.Branch,
		})
		if err != nil {
			return errors.Wrapf(err, "git checkout %s failed in %s", *repository.Branch, repository.URL)
		}

		codemodBranch := uuid.New().String()

		err = repo.Checkout(github.CheckoutOptions{
			Branch: codemodBranch,
			Create: true,
			Force:  true,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		originalDir, err := os.Getwd()
		if err != nil {
			return errors.WithStack(err)
		}

		if err := os.Chdir(tempFolder); err != nil {
			return errors.WithStack(err)
		}

		for _, mod := range applier.codemods {
			if f, ok := mod.Transform.(func(codemod.Project)); ok {
				f(codemod.Project{})
			}
		}

		err = applyCodemodsToDirectory(tempFolder, applier.args.Replacements, applier.codemods)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := os.Chdir(originalDir); err != nil {
			return errors.WithStack(err)
		}

		err = repo.Add(github.AddOptions{
			All: true,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		filesAffected, err := repo.FilesAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		if len(filesAffected) == 0 {
			fmt.Printf("%s %s\n", color.RedString("[NOT CHANGED]"), repository.URL)
			continue
		}

		err = repo.Commit(
			"applied codemods",
			github.CommitOptions{All: true},
		)
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Push()
		if err != nil {
			return errors.WithStack(err)
		}

		pullRequest, err := githubClient.PullRequest(github.PullRequestOptions{
			RepoURL:     repository.URL,
			Title:       "[AUTO GENERATED] applied codemods",
			FromBranch:  codemodBranch,
			ToBranch:    *repository.Branch,
			Description: applier.buildPullRequestDescription(),
		})
		if err != nil {
			return errors.WithStack(err)
		}

		fmt.Printf("%s %s\n", color.GreenString("[CREATED]"), *pullRequest.HTMLURL)
	}

	return nil
}

// Applies codemods to a local directory.
func (applier *Applier) applyCodemodsLocally(ctx context.Context) error {
	originalDir, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := os.Chdir(*applier.args.LocalDirectory); err != nil {
		return errors.WithStack(err)
	}

	for _, mod := range applier.codemods {
		if f, ok := mod.Transform.(func(codemod.Project)); ok {
			f(codemod.Project{})
		}
	}

	if err := applyCodemodsToDirectory(*applier.args.LocalDirectory, applier.args.Replacements, applier.codemods); err != nil {
		return errors.WithStack(err)
	}

	if err := os.Chdir(originalDir); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (applier *Applier) buildPullRequestDescription() string {
	builder := strings.Builder{}

	if len(applier.args.Replacements) > 0 {
		builder.WriteString("Applied the following replacements: \n\n")

		for target, replacement := range applier.args.Replacements {
			builder.WriteString(target)
			builder.WriteString(" => ")
			builder.WriteString(replacement)
		}
	}

	if len(applier.codemods) > 0 {
		builder.WriteString("Applied the following codemods:\n\n")

		for i, codemod := range applier.codemods {
			builder.WriteString(fmt.Sprintf("λ %s", codemod.Description))

			if i < len(applier.codemods)-1 {
				builder.WriteString("\n\n")
			}
		}
	}

	return builder.String()
}
