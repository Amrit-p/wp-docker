package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"wpdock/src/prefix"
)

const (
	network     = "wpdock"
	proxyLabel  = "wpdock.role=proxy"
	namePrefix  = "wpdock-"
	labelPrefix = "wpdock."
	helperImage = "busybox"

	certbotImage = "certbot/certbot"
	sslTemplate  = "site-ssl.conf.tmpl"
)

type Config struct {
	Name    string
	Domain  string
	Aliases string
	Type    string
	Version string
	PHP     string
	Memory  string
	CPU     string
	PIDs    string
	DBHost  string
	DBName  string
	DBUser  string
	DBPass  string
	SSHPort string
	SSHUser string
	SSHPass string
}

type opts struct {
	fs     *flag.FlagSet
	prefix *string
	cfg    *Config
}

func options(name string) *opts {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cfg := &Config{}

	prefix := fs.String("prefix", ".", "directory wpdock was installed into (default: current directory)")
	fs.StringVar(&cfg.Name, "name", "", "site name, used for the container and files")
	fs.StringVar(&cfg.Domain, "domain", "", "primary domain, the server_name nginx routes")
	fs.StringVar(&cfg.Aliases, "aliases", "", "comma-separated extra domains")
	fs.StringVar(&cfg.Type, "type", "", "wordpress or php")
	fs.StringVar(&cfg.Version, "wp-version", "", "wordpress version, e.g. 6.8 (wordpress only)")
	fs.StringVar(&cfg.PHP, "php-version", "", "php version, e.g. 8.3")
	fs.StringVar(&cfg.Memory, "memory", "512m", "memory cap (default 512m)")
	fs.StringVar(&cfg.CPU, "cpu", "0.5", "cpu quota (default 0.5)")
	fs.StringVar(&cfg.PIDs, "pids", "100", "cap on processes in the container (default 100)")
	fs.StringVar(&cfg.DBHost, "db-host", "", "shared mariadb container the site connects to")
	fs.StringVar(&cfg.DBName, "db-name", "", "database the site connects to")
	fs.StringVar(&cfg.DBUser, "db-user", "", "database user the site connects as")
	fs.StringVar(&cfg.DBPass, "db-password", "", "that user's password")

	return &opts{fs: fs, prefix: prefix, cfg: cfg}
}

func (o *opts) root() (string, error) {
	return prefix.Resolve(*o.prefix)
}

func (o *opts) set(name string) bool {
	seen := false
	o.fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func parse(fs *flag.FlagSet, args []string) (bool, error) {
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return false, nil
	}
	return err == nil, err
}

func fail(cmd string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %v", cmd, err)
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func (c *Config) checkName() error {
	if c.Name == "" {
		return fmt.Errorf("--name is required")
	}
	if !nameRe.MatchString(c.Name) {
		return fmt.Errorf("--name: %q: use letters, digits, - and _", c.Name)
	}
	return nil
}

func (c *Config) validate() error {
	if err := c.checkName(); err != nil {
		return err
	}
	if c.Domain == "" {
		return fmt.Errorf("--domain is required")
	}
	switch c.Type {
	case "wordpress", "php":
	case "":
		return fmt.Errorf("--type is required (wordpress or php)")
	default:
		return fmt.Errorf("--type: %q: want wordpress or php", c.Type)
	}
	if c.PHP == "" {
		return fmt.Errorf("--php-version is required")
	}
	if c.Type == "wordpress" {
		if c.Version == "" {
			return fmt.Errorf("--wp-version is required for a wordpress site")
		}
		for _, f := range []struct{ flag, value string }{
			{"--db-host", c.DBHost},
			{"--db-name", c.DBName},
			{"--db-user", c.DBUser},
			{"--db-password", c.DBPass},
		} {
			if f.value == "" {
				return fmt.Errorf("%s is required for a wordpress site", f.flag)
			}
		}
	}
	return nil
}

func (c *Config) baseImage() (string, error) {
	switch c.Type {
	case "wordpress":
		return fmt.Sprintf("wordpress:%s-php%s-apache", c.Version, c.PHP), nil
	case "php":
		return fmt.Sprintf("php:%s-apache", c.PHP), nil
	}
	return "", fmt.Errorf("--type: %q: want wordpress or php", c.Type)
}

func prepareImage(c *Config) (string, error) {
	base, err := c.baseImage()
	if err != nil {
		return "", err
	}
	if c.SSHPort == "" {
		return base, nil
	}
	if err := ensureImage(base); err != nil {
		return "", err
	}
	if err := ensureSSHImage(base); err != nil {
		return "", err
	}
	return sshImage(base), nil
}

func sshImage(base string) string {
	return "wpdock-ssh:" + strings.NewReplacer(":", "-", "/", "-").Replace(base)
}

func ensureSSHImage(base string) error {
	tag := sshImage(base)
	if err := docker("image", "inspect", tag); err == nil {
		return nil
	}

	ctx, err := os.MkdirTemp("", "wpdock-ssh-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(ctx)

	if err := os.WriteFile(filepath.Join(ctx, "entrypoint.sh"), []byte(sshEntrypoint), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ctx, "Dockerfile"), []byte("FROM "+base+"\n"+sshDockerfile), 0o644); err != nil {
		return err
	}

	return docker("build", "-t", tag, ctx)
}

const sshDockerfile = `RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends openssh-server sudo curl less; \
    rm -rf /var/lib/apt/lists/*; \
    mkdir -p /run/sshd; \
    printf 'PasswordAuthentication yes\nPermitRootLogin yes\n' >> /etc/ssh/sshd_config; \
    curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar; \
    chmod +x /usr/local/bin/wp; \
    curl -fsSL https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer
COPY entrypoint.sh /usr/local/bin/wpdock-sshd
RUN chmod +x /usr/local/bin/wpdock-sshd
ENTRYPOINT ["/usr/local/bin/wpdock-sshd"]
CMD ["apache2-foreground"]
`

const sshEntrypoint = `#!/bin/sh
if [ -n "$WPDOCK_SSH_USER" ]; then
	id "$WPDOCK_SSH_USER" >/dev/null 2>&1 || useradd -o -u 33 -g 33 -m -s /bin/bash "$WPDOCK_SSH_USER" 2>/dev/null
	echo "$WPDOCK_SSH_USER:$WPDOCK_SSH_PASSWORD" | chpasswd
	printf '#33 ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/wpdock
	chmod 0440 /etc/sudoers.d/wpdock
fi
ssh-keygen -A >/dev/null 2>&1
mkdir -p /run/sshd
/usr/sbin/sshd
if command -v docker-entrypoint.sh >/dev/null 2>&1; then
	exec docker-entrypoint.sh "$@"
fi
exec docker-php-entrypoint "$@"
`

func container(name string) string           { return namePrefix + name }
func dataDir(root, name string) string       { return filepath.Join(root, "data", name) }
func certLive(root, domain string) string    { return filepath.Join(root, "certs", "live", domain) }
func vhostPath(root, name string) string     { return filepath.Join(root, "nginx", "conf", name+".conf") }
func backupPath(root, id, ext string) string { return filepath.Join(root, "backups", id+ext) }

func command(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(out.String()); msg != "" {
			return fmt.Errorf("%s %s: %v: %s", bin, args[0], err, msg)
		}
		return fmt.Errorf("%s %s: %v", bin, args[0], err)
	}
	return nil
}

func docker(args ...string) error { return command("docker", args...) }

func dockerOut(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)

	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errs.String()); msg != "" {
			return "", fmt.Errorf("docker %s: %v: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("docker %s: %v", args[0], err)
	}
	return out.String(), nil
}

func ensureNetwork() error {
	if err := docker("network", "inspect", network); err == nil {
		return nil
	}
	return docker("network", "create", network)
}

func siteExists(name string) bool {
	return docker("inspect", container(name)) == nil
}

func ensurePortFree(port, own string) error {
	if port == "" {
		return nil
	}

	out, err := dockerOut("ps", "--format", "{{.Names}}\t{{.Ports}}")
	if err != nil {
		return err
	}

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, ports, ok := strings.Cut(line, "\t")
		if !ok || name == own {
			continue
		}
		if strings.Contains(ports, ":"+port+"->") {
			return fmt.Errorf("host port %s is already in use by container %q", port, name)
		}
	}
	return nil
}

func dbEnvPairs(c *Config) []string {
	return []string{
		"WORDPRESS_DB_HOST=" + c.DBHost,
		"WORDPRESS_DB_NAME=" + c.DBName,
		"WORDPRESS_DB_USER=" + c.DBUser,
		"WORDPRESS_DB_PASSWORD=" + c.DBPass,
	}
}

func dbEnvArgs(c *Config) []string {
	if c.DBName == "" {
		return nil
	}
	args := make([]string, 0, 8)
	for _, p := range dbEnvPairs(c) {
		args = append(args, "-e", p)
	}
	return args
}

func labelArgs(c *Config) []string {
	pairs := [][2]string{
		{"managed", "true"},
		{"type", c.Type},
		{"domain", c.Domain},
		{"aliases", c.Aliases},
		{"version", c.Version},
		{"php", c.PHP},
		{"memory", c.Memory},
		{"cpu", c.CPU},
		{"pids", c.PIDs},
		{"db-host", c.DBHost},
		{"db-name", c.DBName},
		{"db-user", c.DBUser},
		{"ssh-port", c.SSHPort},
		{"ssh-user", c.SSHUser},
	}

	args := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		args = append(args, "--label", labelPrefix+p[0]+"="+p[1])
	}
	return args
}

func run(root string, c *Config, img string) error {
	if err := os.MkdirAll(dataDir(root, c.Name), 0o755); err != nil {
		return err
	}

	args := []string{
		"run", "-d",
		"--name", container(c.Name),
		"--restart", "unless-stopped",
		"--network", network,
	}
	if c.Memory != "" {
		args = append(args, "--memory", c.Memory)
	}
	if c.CPU != "" {
		args = append(args, "--cpus", c.CPU)
	}
	if c.PIDs != "" {
		args = append(args, "--pids-limit", c.PIDs)
	}
	args = append(args, "-v", dataDir(root, c.Name)+":/var/www/html")
	args = append(args, dbEnvArgs(c)...)
	args = append(args, sshArgs(c)...)
	args = append(args, labelArgs(c)...)
	args = append(args, img)

	return docker(args...)
}

func sshArgs(c *Config) []string {
	if c.SSHPort == "" {
		return nil
	}
	return []string{
		"-p", c.SSHPort + ":22",
		"-e", "WPDOCK_SSH_USER=" + c.SSHUser,
		"-e", "WPDOCK_SSH_PASSWORD=" + c.SSHPass,
	}
}

func inspectContainer(fullName, format string, v any) error {
	out, err := dockerOut("inspect", "--format", format, fullName)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(strings.TrimSpace(out)), v)
}

func inspectJSON(name, format string, v any) error {
	return inspectContainer(container(name), format, v)
}

func inspect(name string) (*Config, error) {
	labels := map[string]string{}
	if err := inspectJSON(name, "{{json .Config.Labels}}", &labels); err != nil {
		return nil, err
	}
	if labels[labelPrefix+"managed"] != "true" {
		return nil, fmt.Errorf("%s is not a wpdock-managed site", container(name))
	}

	var env []string
	if err := inspectJSON(name, "{{json .Config.Env}}", &env); err != nil {
		return nil, err
	}

	return &Config{
		Name:    name,
		Type:    labels[labelPrefix+"type"],
		Domain:  labels[labelPrefix+"domain"],
		Aliases: labels[labelPrefix+"aliases"],
		Version: labels[labelPrefix+"version"],
		PHP:     labels[labelPrefix+"php"],
		Memory:  labels[labelPrefix+"memory"],
		CPU:     labels[labelPrefix+"cpu"],
		PIDs:    labels[labelPrefix+"pids"],
		DBHost:  labels[labelPrefix+"db-host"],
		DBName:  labels[labelPrefix+"db-name"],
		DBUser:  labels[labelPrefix+"db-user"],
		DBPass:  envValue(env, "WORDPRESS_DB_PASSWORD"),
		SSHPort: labels[labelPrefix+"ssh-port"],
		SSHUser: labels[labelPrefix+"ssh-user"],
		SSHPass: envValue(env, "WPDOCK_SSH_PASSWORD"),
	}, nil
}

func envValue(env []string, key string) string {
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func serverName(c *Config) string {
	names := []string{c.Domain}
	for _, a := range strings.Split(c.Aliases, ",") {
		if a = strings.TrimSpace(a); a != "" {
			names = append(names, a)
		}
	}
	return strings.Join(names, " ")
}

type vhost struct {
	Name       string
	ServerName string
	Domain     string
	Upstream   string
	Prefix     string
}

// hasCert reports whether the site's domain holds a certificate. The check is
// certbot's own renewal config rather than live/<domain>/, because certbot
// keeps live/ root-only while renewal/ stays readable to the user running
// wpdock. The certificate on disk is the whole of a site's ssl state: no
// label or flag records it, so update and restore keep https automatically.
func hasCert(root, domain string) bool {
	return exists(filepath.Join(root, "certs", "renewal", domain+".conf"))
}

func writeVhost(root string, c *Config) error {
	tmpl := "site.conf.tmpl"
	if hasCert(root, c.Domain) {
		tmpl = sslTemplate
	}

	b, err := os.ReadFile(filepath.Join(root, "nginx", "templates", tmpl))
	if err != nil {
		return err
	}

	t, err := template.New("site").Parse(string(b))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, vhost{
		Name:       c.Name,
		ServerName: serverName(c),
		Domain:     c.Domain,
		Upstream:   container(c.Name),
		Prefix:     root,
	}); err != nil {
		return err
	}

	path := vhostPath(root, c.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func removeVhost(root, name string) error {
	if err := os.Remove(vhostPath(root, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func reload() error {
	out, err := dockerOut("ps", "--filter", "label="+proxyLabel, "--format", "{{.Names}}")
	if err != nil {
		return err
	}

	names := strings.Fields(out)
	if len(names) == 0 {
		return fmt.Errorf("no running proxy container found (label %s) — is the compose stack up?", proxyLabel)
	}

	for _, name := range names {
		if err := docker("kill", "--signal=HUP", name); err != nil {
			return err
		}
	}
	return nil
}

func publish(root string, c *Config) error {
	if err := writeVhost(root, c); err != nil {
		return err
	}
	return reload()
}

func field(key, value string) {
	fmt.Printf("  %-11s %s\n", key, value)
}

func describeCore(c *Config) {
	fmt.Printf("site %s\n\n", c.Name)
	field("type", c.Type)
	field("domain", serverName(c))
	if c.Type == "wordpress" {
		field("wordpress", c.Version)
	}
	field("php", c.PHP)
	field("resources", fmt.Sprintf("memory=%s cpus=%s max_pids=%s", c.Memory, c.CPU, c.PIDs))
}

func describe(c *Config) {
	describeCore(c)
	if c.DBName != "" {
		field("database", fmt.Sprintf("%s@%s as %s", c.DBName, c.DBHost, c.DBUser))
	}
}

type manifest struct {
	ID     string  `json:"id"`
	Image  string  `json:"image"`
	Config *Config `json:"config"`
}

func writeManifest(root string, m manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	path := backupPath(root, m.ID, ".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func readManifest(root, id string) (manifest, error) {
	b, err := os.ReadFile(backupPath(root, id, ".json"))
	if err != nil {
		return manifest{}, err
	}

	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

func ensureImage(img string) error {
	if err := docker("image", "inspect", img); err == nil {
		return nil
	}
	return docker("pull", img)
}

func helper(root string, extra []string, sh string) error {
	if err := ensureImage(helperImage); err != nil {
		return err
	}
	args := append([]string{"run", "--rm", "-v", filepath.Join(root, "data") + ":/data"}, extra...)
	args = append(args, helperImage, "sh", "-c", sh)
	return docker(args...)
}

func archive(root, name, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return helper(root, []string{"-v", filepath.Dir(out) + ":/out"},
		fmt.Sprintf("tar czf /out/%s -C /data %s", filepath.Base(out), name))
}

func extract(root, name, in string) error {
	return helper(root, []string{"-v", filepath.Dir(in) + ":/in:ro"},
		fmt.Sprintf("rm -rf /data/%s && tar xzf /in/%s -C /data", name, filepath.Base(in)))
}

func removeData(root, name string) error {
	return helper(root, nil, fmt.Sprintf("rm -rf /data/%s", name))
}
