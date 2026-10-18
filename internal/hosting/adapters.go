package hosting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type packageJSON struct {
	Name            string            `json:"name"`
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func sourceBase(p Project) string {
	if p.BaseDirectory == "" {
		return "."
	}
	return p.BaseDirectory
}
func pathFor(p Project, s string) string { return path.Join(sourceBase(p), s) }
func detectPackage(p packageJSON) string {
	for _, entry := range []struct{ dependency, adapter string }{{"next", "next"}, {"nuxt", "nuxt"}, {"@nestjs/core", "node"}, {"express", "node"}, {"vite", "static"}} {
		if p.Dependencies[entry.dependency] != "" || p.DevDependencies[entry.dependency] != "" {
			return entry.adapter
		}
	}
	if p.Scripts["start"] != "" || p.Main != "" {
		return "node"
	}
	return "static"
}
func detectLocal(root string) string {
	raw, e := os.ReadFile(filepath.Join(root, "package.json"))
	if e != nil {
		return "static files"
	}
	var p packageJSON
	if json.Unmarshal(raw, &p) != nil {
		return "unrecognised package.json"
	}
	return detectPackage(p)
}
func (a *App) detectGitHub(ctx context.Context, p Project) (map[string]any, error) {
	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	endpoint := "/contents/" + pathFor(p, "package.json") + "?ref=" + p.Branch
	if e := a.githubCall(ctx, p, endpoint, &file); e != nil {
		return nil, e
	}
	if file.Encoding != "base64" {
		return nil, errors.New("GitHub did not return a regular package.json file")
	}
	raw, e := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if e != nil {
		return nil, e
	}
	var pkg packageJSON
	if e = json.Unmarshal(raw, &pkg); e != nil {
		return nil, errors.New("Source package.json is invalid")
	}
	adapter := detectPackage(pkg)
	install := []string{"npm", "ci"}
	build := []string{}
	start := []string{}
	output := "dist"
	message := "Commit a package-lock.json so dependency installation can use npm ci."
	if pkg.Scripts["build"] != "" {
		build = []string{"npm", "run", "build"}
	}
	if pkg.Scripts["start"] != "" {
		start = []string{"npm", "run", "start"}
	}
	if adapter == "next" {
		start = []string{}
		message += " Next.js requires output:'standalone' in next.config; static/public assets are copied into its standalone output."
	}
	if adapter == "nuxt" {
		start = []string{}
		message += " Nuxt requires a Nitro node-server build."
	}
	if adapter == "node" && len(start) == 0 && relPath(pkg.Main) {
		start = []string{"node", pkg.Main}
	}
	return map[string]any{"adapter": adapter, "install": install, "build": build, "start": start, "output": output, "message": message, "packageName": pkg.Name}, nil
}
func copyArtifactTree(source, dest string) error {
	if _, e := os.Stat(source); os.IsNotExist(e) {
		return nil
	}
	return filepath.WalkDir(source, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("Next output contains an unsupported link")
		}
		rel, e := filepath.Rel(source, p)
		if e != nil {
			return e
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		in, e := os.Open(p)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		ce := out.Close()
		if e != nil {
			return e
		}
		return ce
	})
}
func prepareNextOutput(root string) error {
	standalone := filepath.Join(root, ".next", "standalone")
	if _, e := os.Stat(filepath.Join(standalone, "server.js")); e != nil {
		return errors.New("Next standalone output is missing. Set output:'standalone' and run the build")
	}
	if e := copyArtifactTree(filepath.Join(root, ".next", "static"), filepath.Join(standalone, ".next", "static")); e != nil {
		return e
	}
	return copyArtifactTree(filepath.Join(root, "public"), filepath.Join(standalone, "public"))
}
