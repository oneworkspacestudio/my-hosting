package hosting

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const Version = "0.1.0-preview.1"

type Config struct{ Data, Listen, Origin, Issuer, ClientID, ClientSecret, Owner string }
type User struct {
	ID      string `json:"id"`
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
}
type Project struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Slug       string   `json:"slug"`
	Repository string   `json:"repository"`
	Branch     string   `json:"branch"`
	Adapter    string   `json:"adapter"`
	NodeMajor  int      `json:"nodeMajor"`
	Install    []string `json:"install"`
	Build      []string `json:"build"`
	Start      []string `json:"start"`
	Output     string   `json:"output"`
	HealthPath string   `json:"healthPath"`
	Timeout    int      `json:"timeout"`
	MemoryMB   int      `json:"memoryMB"`
	DiskMB     int      `json:"diskMB"`
	Previews   bool     `json:"previews"`
	Trusted    bool     `json:"trusted"`
	Live       string   `json:"live"`
	Created    string   `json:"created"`
	Role       string   `json:"role,omitempty"`
}
type Deploy struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"projectId"`
	State       string  `json:"state"`
	Environment string  `json:"environment"`
	Commit      string  `json:"commit"`
	Branch      string  `json:"branch"`
	Author      string  `json:"author"`
	Message     string  `json:"message"`
	Source      string  `json:"source"`
	SourceHash  string  `json:"sourceHash"`
	Parent      string  `json:"parent"`
	Artifact    string  `json:"artifact"`
	Action      string  `json:"action"`
	Created     string  `json:"created"`
	Started     string  `json:"started"`
	Finished    string  `json:"finished"`
	Error       string  `json:"error"`
	URL         string  `json:"url"`
	Bytes       int64   `json:"bytes"`
	Port        int     `json:"port"`
	Snapshot    Project `json:"config"`
}
type Domain struct {
	Host      string `json:"host"`
	ProjectID string `json:"projectId"`
	Token     string `json:"token"`
	Verified  bool   `json:"verified"`
	HTTPS     string `json:"https"`
	Error     string `json:"error"`
	Canonical bool   `json:"canonical"`
}
type Settings struct {
	CaddyPort     int    `json:"caddyPort"`
	Public        bool   `json:"public"`
	PreviewDomain string `json:"previewDomain"`
	Retain        int    `json:"retain"`
	Concurrency   int    `json:"concurrency"`
}

func defaults() Settings { return Settings{CaddyPort: 9080, Retain: 10, Concurrency: 1} }
func now() string        { return time.Now().UTC().Format(time.RFC3339Nano) }
func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func encode(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return string(b)
}

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var validSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{1,47}$`)
var validRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)
var validSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)
var validBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./-]{0,150}$`)

func cleanRepo(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), "/"), ".git")
	s = strings.TrimPrefix(s, "https://github.com/")
	if !validRepo.MatchString(s) || strings.Contains(s, "..") {
		return "", errors.New("Use a GitHub owner/repository or https://github.com/owner/repository URL")
	}
	return s, nil
}
func relPath(s string) bool {
	if s == "." {
		return true
	}
	return s != "" && !filepath.IsAbs(s) && !strings.ContainsAny(s, ":\\\x00") && !strings.HasPrefix(s, "/") && !strings.Contains("/"+s+"/", "/../") && !strings.Contains("/"+s+"/", "/./")
}
func (p *Project) validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if len(p.Name) < 1 || len(p.Name) > 100 || !validSlug.MatchString(p.Slug) {
		return errors.New("Enter a project name and a lowercase site name (2–48 letters, numbers or hyphens)")
	}
	if p.Repository != "" {
		r, e := cleanRepo(p.Repository)
		if e != nil {
			return e
		}
		p.Repository = r
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	if !validBranch.MatchString(p.Branch) || strings.Contains(p.Branch, "..") {
		return errors.New("Invalid production branch")
	}
	switch p.Adapter {
	case "static", "node", "next", "nuxt":
	default:
		return errors.New("Choose static, node, next or nuxt")
	}
	if p.NodeMajor == 0 {
		p.NodeMajor = 24
	}
	if p.NodeMajor != 22 && p.NodeMajor != 24 {
		return errors.New("This release supports pinned Node 22 and 24 runtimes")
	}
	if p.Output == "" {
		p.Output = "dist"
	}
	if !relPath(p.Output) {
		return errors.New("Output directory must stay inside the release")
	}
	if p.HealthPath == "" {
		p.HealthPath = "/"
	}
	u, e := url.Parse(p.HealthPath)
	if e != nil || !strings.HasPrefix(p.HealthPath, "/") || strings.HasPrefix(p.HealthPath, "//") || u.Host != "" || u.Fragment != "" || strings.ContainsAny(p.HealthPath, "\r\n") {
		return errors.New("Health path must be a local /path")
	}
	if p.Timeout == 0 {
		p.Timeout = 600
	}
	if p.MemoryMB == 0 {
		p.MemoryMB = 1024
	}
	if p.DiskMB == 0 {
		p.DiskMB = 1024
	}
	if p.Timeout < 10 || p.Timeout > 1800 || p.MemoryMB < 128 || p.MemoryMB > 4096 || p.DiskMB < 10 || p.DiskMB > 10240 {
		return errors.New("Build limits: 10–1800 seconds, 128–4096 MiB RAM and 10–10240 MiB disk")
	}
	for _, args := range [][]string{p.Install, p.Build, p.Start} {
		if len(args) > 30 {
			return errors.New("Too many command arguments")
		}
		for _, arg := range args {
			if len(arg) > 1000 || strings.ContainsRune(arg, 0) {
				return errors.New("Invalid command argument")
			}
		}
		if len(args) > 0 && args[0] != "node" && args[0] != "npm" {
			return errors.New("Commands use explicit JSON argument arrays beginning with node or npm; shell commands are not accepted")
		}
	}
	if (len(p.Install) > 0 || len(p.Build) > 0 || p.Adapter != "static") && !p.Trusted {
		return errors.New("An administrator must explicitly approve this project's source and executable scripts")
	}
	return nil
}
func validDomain(s string) bool {
	if len(s) > 253 || s != strings.ToLower(s) || net.ParseIP(s) != nil || !strings.Contains(s, ".") || strings.HasSuffix(s, ".localhost") || strings.HasSuffix(s, ".local") {
		return false
	}
	for _, x := range strings.Split(s, ".") {
		if !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`).MatchString(x) {
			return false
		}
	}
	return true
}
