package hosting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type repositoryLocation struct{ Provider, Base, Name, URL string }
type repositoryInfo struct {
	Provider      string `json:"provider"`
	Repository    string `json:"repository"`
	FullName      string `json:"fullName"`
	ID            int64  `json:"id"`
	DefaultBranch string `json:"defaultBranch"`
	Private       bool   `json:"private"`
	Empty         bool   `json:"empty"`
}

var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func parseRepository(raw, myGit string) (repositoryLocation, error) {
	s := strings.TrimSpace(raw)
	if len(s) > 2048 || strings.ContainsAny(s, "\\\r\n\x00") {
		return repositoryLocation{}, errors.New("Enter a repository name or a valid HTTPS repository URL")
	}
	if strings.HasPrefix(s, "git@") {
		host, repo, ok := strings.Cut(strings.TrimPrefix(s, "git@"), ":")
		b, _ := url.Parse(myGit)
		if !ok {
			return repositoryLocation{}, errors.New("Use the HTTPS clone URL")
		}
		if strings.EqualFold(host, "github.com") {
			s = "https://github.com/" + repo
		} else if b != nil && strings.EqualFold(host, b.Hostname()) {
			s = strings.TrimRight(myGit, "/") + "/" + repo
		} else {
			return repositoryLocation{}, errors.New("Use the HTTPS clone URL for this Git server")
		}
	}
	if !strings.Contains(s, "://") {
		s = strings.TrimSuffix(strings.TrimRight(s, "/"), ".git")
		parts := strings.Split(s, "/")
		if len(parts) < 1 || len(parts) > 2 {
			return repositoryLocation{}, errors.New("Use a repository name, owner/repository, or its HTTPS URL")
		}
		for _, v := range parts {
			if !repositorySegment.MatchString(v) || v == "." || v == ".." {
				return repositoryLocation{}, errors.New("Invalid repository name")
			}
		}
		if myGit != "" {
			return repositoryLocation{"mygit", strings.TrimRight(myGit, "/"), s, ""}, nil
		}
		if len(parts) == 1 {
			return repositoryLocation{}, errors.New("Open Hosting through One Workspace to find My Git repositories by name, or paste a complete repository URL")
		}
		return repositoryLocation{"github", "https://github.com", s, "https://github.com/" + s}, nil
	}
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.Path, "//") {
		return repositoryLocation{}, errors.New("Use an HTTPS repository URL without credentials, query parameters or fragments")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), ".git"), "/"), "/")
	if len(parts) < 2 {
		return repositoryLocation{}, errors.New("Repository URLs include both an owner and a repository")
	}
	for _, v := range parts {
		if !repositorySegment.MatchString(v) || v == "." || v == ".." {
			return repositoryLocation{}, errors.New("Invalid repository URL path")
		}
	}
	name := strings.Join(parts[len(parts)-2:], "/")
	base := u.Scheme + "://" + strings.ToLower(u.Host)
	if len(parts) > 2 {
		base += "/" + strings.Join(parts[:len(parts)-2], "/")
	}
	if strings.EqualFold(u.Host, "github.com") && len(parts) == 2 && u.Scheme == "https" {
		return repositoryLocation{"github", "https://github.com", name, "https://github.com/" + name}, nil
	}
	b, _ := url.Parse(myGit)
	if b != nil && strings.EqualFold(u.Host, b.Host) && u.Scheme == b.Scheme && base == strings.TrimRight(myGit, "/") {
		return repositoryLocation{"mygit", base, name, base + "/" + name}, nil
	}
	if u.Scheme != "https" || u.Port() != "" && u.Port() != "443" || !publicRepositoryIP(net.ParseIP(u.Hostname())) && net.ParseIP(u.Hostname()) != nil || strings.EqualFold(u.Hostname(), "localhost") {
		return repositoryLocation{}, errors.New("Use this Workspace's My Git URL or a public HTTPS Git server")
	}
	return repositoryLocation{"gitea", base, name, base + "/" + name}, nil
}

// Direct external servers cannot turn a repository field into an internal HTTP
// proxy. The trusted My Git broker uses its separate, loopback-only transport.
func publicRepositoryIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
func externalRepositoryClient(base string) *http.Client {
	t := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("Git server address is unavailable")
		}
		for _, ip := range ips {
			if !publicRepositoryIP(ip.IP) {
				return nil, errors.New("Private network Git servers must use the Workspace My Git connection")
			}
		}
		var last error
		for _, ip := range ips {
			c, e := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}}
	b, _ := url.Parse(base)
	return &http.Client{Timeout: 2 * time.Minute, Transport: t, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 4 || r.URL.Scheme != b.Scheme || !strings.EqualFold(r.URL.Host, b.Host) || !strings.HasPrefix(r.URL.Path, strings.TrimRight(b.Path, "/")+"/") {
			return errors.New("Git server redirected outside its trusted address")
		}
		return nil
	}}
}

type gitBridgeCall struct {
	Operation    string `json:"operation"`
	Issuer       string `json:"issuer"`
	Subject      string `json:"subject"`
	Repository   string `json:"repository"`
	RepositoryID int64  `json:"repositoryId,omitempty"`
	Ref          string `json:"ref,omitempty"`
	File         string `json:"file,omitempty"`
	Page         int    `json:"page,omitempty"`
}
type repositoryHTTPError struct {
	Status  int
	Message string
}

func (e *repositoryHTTPError) Error() string { return e.Message }

func (a *App) gitBridge(ctx context.Context, p Project, in gitBridgeCall) (*http.Response, error) {
	u, e := url.Parse(a.cfg.GitBridgeURL)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/_integration/hosting/git" || !net.ParseIP(u.Hostname()).IsLoopback() || a.cfg.GitBridgeToken == "" {
		return nil, errors.New("Update One Workspace to connect Hosting with My Git automatically")
	}
	var actor User
	if e = a.store.db.QueryRowContext(ctx, "SELECT issuer,subject,role FROM users WHERE id=?", p.SourceActor).Scan(&actor.Issuer, &actor.Subject, &actor.Role); e != nil || actor.Role == "disabled" || actor.Issuer != a.cfg.Issuer {
		return nil, errors.New("Reconnect this project's source using your MyAccounts sign-in")
	}
	in.Issuer = actor.Issuer
	in.Subject = actor.Subject
	in.Repository = p.Repository
	in.RepositoryID = p.SourceRepoID
	r, e := http.NewRequestWithContext(ctx, "POST", a.cfg.GitBridgeURL, strings.NewReader(encode(in)))
	if e != nil {
		return nil, e
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+a.cfg.GitBridgeToken)
	c := &http.Client{Timeout: 2 * time.Minute, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := c.Do(r)
	if e != nil {
		return nil, errors.New("My Git connection is unavailable; retry shortly")
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &repositoryHTTPError{resp.StatusCode, strings.TrimSpace(string(b))}
	}
	return resp, nil
}

func (a *App) resolveRepository(ctx context.Context, raw string, u User) (repositoryInfo, error) {
	loc, e := parseRepository(raw, a.cfg.GitBase)
	if e != nil {
		return repositoryInfo{}, e
	}
	if loc.Provider != "mygit" {
		return repositoryInfo{Provider: loc.Provider, Repository: loc.URL, FullName: loc.Name}, nil
	}
	p := Project{Repository: raw, SourceActor: u.ID}
	r, e := a.gitBridge(ctx, p, gitBridgeCall{Operation: "resolve"})
	if e != nil {
		return repositoryInfo{}, e
	}
	defer r.Body.Close()
	var info repositoryInfo
	if e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&info); e != nil {
		return info, errors.New("My Git repository details are unavailable")
	}
	resolved, e := parseRepository(info.Repository, a.cfg.GitBase)
	if e != nil || resolved.Provider != "mygit" || info.Provider != "mygit" || info.ID <= 0 || !validBranch.MatchString(info.DefaultBranch) && !info.Empty {
		return info, errors.New("My Git returned invalid repository details")
	}
	return info, nil
}

func (a *App) prepareProjectRepository(ctx context.Context, p *Project, u User, previous *Project) error {
	if previous != nil && strings.TrimSpace(p.Repository) == previous.Repository {
		p.SourceProvider = previous.SourceProvider
		p.SourceRepoID = previous.SourceRepoID
		p.SourceActor = previous.SourceActor
		return nil
	}
	p.SourceProvider = ""
	p.SourceRepoID = 0
	p.SourceActor = ""
	if strings.TrimSpace(p.Repository) == "" {
		p.Repository = ""
		return nil
	}
	info, e := a.resolveRepository(ctx, p.Repository, u)
	if e != nil {
		return e
	}
	p.Repository = info.Repository
	p.SourceProvider = info.Provider
	if info.Provider == "github" {
		p.Repository = info.FullName
	}
	if info.Provider == "mygit" {
		p.SourceActor = u.ID
		p.SourceRepoID = info.ID
		if p.Branch == "" {
			p.Branch = info.DefaultBranch
		}
	}
	return nil
}

func repositoryProvider(p Project) string {
	if p.SourceProvider == "" {
		return "github"
	}
	return p.SourceProvider
}

func bridgeEndpoint(endpoint string) (gitBridgeCall, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host != "" || u.Scheme != "" {
		return gitBridgeCall{}, errors.New("Invalid source operation")
	}
	q := u.Query()
	in := gitBridgeCall{}
	switch {
	case u.Path == "":
		in.Operation = "metadata"
	case u.Path == "/branches":
		in.Operation = "branches"
	case strings.HasPrefix(u.Path, "/commits/"):
		in.Operation = "commit"
		in.Ref = strings.TrimPrefix(u.Path, "/commits/")
	case u.Path == "/commits":
		in.Operation = "commits"
		in.Ref = q.Get("sha")
		in.Page, _ = strconv.Atoi(q.Get("page"))
	case strings.HasPrefix(u.Path, "/contents/"):
		in.Operation = "contents"
		in.File = strings.TrimPrefix(u.Path, "/contents/")
		in.Ref = q.Get("ref")
	default:
		return in, errors.New("Unsupported repository read")
	}
	return in, nil
}

func (a *App) repositoryCredential(p Project) string {
	if repositoryProvider(p) == "github" {
		v, _ := a.getSecret(p.ID, "GITHUB_TOKEN", "integration")
		return v
	}
	scope, _ := a.getSecret(p.ID, "REPOSITORY_TOKEN_SCOPE", "integration")
	if scope != p.Repository {
		return ""
	}
	v, _ := a.getSecret(p.ID, "REPOSITORY_TOKEN", "integration")
	return v
}

func (a *App) repositoryResponse(ctx context.Context, p Project, endpoint string) (*http.Response, error) {
	if repositoryProvider(p) == "mygit" {
		in, e := bridgeEndpoint(endpoint)
		if e != nil {
			return nil, e
		}
		return a.gitBridge(ctx, p, in)
	}
	loc, e := parseRepository(p.Repository, "")
	if e != nil {
		return nil, e
	}
	r, e := http.NewRequestWithContext(ctx, "GET", loc.Base+"/api/v1/repos/"+loc.Name+endpoint, nil)
	if e != nil {
		return nil, e
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "OneWorkspace-MyHosting/"+Version)
	if token := a.repositoryCredential(p); token != "" {
		r.Header.Set("Authorization", "token "+token)
	}
	return externalRepositoryClient(loc.Base).Do(r)
}

func (a *App) gitSource(ctx context.Context, p Project, branch, sha string) (string, string, string, error) {
	if branch == "" {
		branch = p.Branch
	}
	if !validBranch.MatchString(branch) || strings.Contains(branch, "..") || sha != "" && !validSHA.MatchString(sha) {
		return "", "", "", errors.New("Invalid branch or immutable commit")
	}
	if branch != p.Branch && !p.Previews {
		return "", "", "", errors.New("Preview branches are disabled for this project")
	}
	type commit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	var head commit
	if e := a.githubCall(ctx, p, "/commits/"+url.PathEscape(branch), &head); e != nil {
		return "", "", "", e
	}
	if !validSHA.MatchString(head.SHA) {
		return "", "", "", errors.New("Git did not return an immutable commit")
	}
	chosen := head
	if sha != "" && sha != head.SHA {
		found := false
		for page := 1; page <= 100; page++ {
			var cs []commit
			e := a.githubCall(ctx, p, "/commits?per_page=100&limit=100&page="+strconv.Itoa(page)+"&sha="+url.QueryEscape(branch), &cs)
			if e != nil {
				return "", "", "", e
			}
			for _, c := range cs {
				if c.SHA == sha {
					chosen = c
					found = true
					break
				}
			}
			if found || len(cs) < 100 {
				break
			}
		}
		if !found {
			return "", "", "", errors.New("The selected commit is not in the allowed branch's recent history; select a current branch commit")
		}
	}
	msg := strings.SplitN(chosen.Commit.Message, "\n", 2)[0]
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return chosen.SHA, chosen.Commit.Author.Name, msg, nil
}

func (a *App) gitArchive(ctx context.Context, p Project, sha, dest string, report func(string)) error {
	var r *http.Response
	var e error
	if repositoryProvider(p) == "mygit" {
		r, e = a.gitBridge(ctx, p, gitBridgeCall{Operation: "archive", Ref: sha})
	} else {
		r, e = a.repositoryResponse(ctx, p, "/archive/"+sha+".tar.gz")
	}
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("Git source download returned HTTP %d; check repository access", r.StatusCode)
	}
	if r.ContentLength > 64<<20 {
		return errors.New("Repository source exceeds 64 MiB")
	}
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	complete := false
	defer func() {
		f.Close()
		if !complete {
			_ = os.Remove(dest)
		}
	}()
	var n int64
	buf := make([]byte, 1<<20)
	next := int64(8 << 20)
	for {
		count, re := r.Body.Read(buf)
		if count > 0 {
			n += int64(count)
			if n > 64<<20 {
				return errors.New("Repository source exceeds 64 MiB")
			}
			if _, e = f.Write(buf[:count]); e != nil {
				return e
			}
			if report != nil && n >= next {
				report(fmt.Sprintf("Downloaded %d MiB of Git source", n>>20))
				next += 8 << 20
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return errors.New("Git source transfer was interrupted; retry the deployment")
		}
	}
	if e = f.Sync(); e != nil {
		return e
	}
	complete = true
	return nil
}
