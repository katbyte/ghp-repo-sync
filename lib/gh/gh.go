package gh

import (
	"context"

	"github.com/google/go-github/v45/github"
	common "github.com/katbyte/ghp-pr-sync/lib/chttp"
	"golang.org/x/oauth2"
)

type Token struct {
	Token *string
}

type Repo struct {
	Owner string
	Name  string
	Token
}

func NewRepo(owner, repo, token string) Repo {
	r := Repo{
		Owner: owner,
		Name:  repo,
		Token: Token{
			Token: nil,
		},
	}

	if token != "" {
		r.Token.Token = &token
	}

	return r
}

type Project struct {
	Owner  string
	Number int
	Token
}

func NewProject(owner string, number int, token string) Project {
	p := Project{
		Owner:  owner,
		Number: number,
		Token: Token{
			Token: nil,
		},
	}

	if token != "" {
		p.Token.Token = &token
	}

	return p
}

func (t Token) NewClient() (*github.Client, context.Context) {
	ctx := context.Background()
	httpClient := common.NewHTTPClient("GitHub")

	if t := t.Token; t != nil {
		t := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: *t},
		)
		httpClient = oauth2.NewClient(ctx, t)
	}

	httpClient.Transport = common.NewTransport("GitHub", httpClient.Transport)

	return github.NewClient(httpClient), ctx
}
