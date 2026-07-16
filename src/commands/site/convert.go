package site

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"wpdock/src/commands/db"
	"wpdock/src/prefix"
	"wpdock/src/prompt"
)

func ConvertUsage() {
	fmt.Fprint(os.Stderr, `  site-convert --prefix=<path> --old-prefix=<path> --name=<site> --root-password=<pass> [--yes]
        copy a site off the old make/bash stack into wpdock: files, database and routing
`)
}

func Convert(args []string) error { return fail("site-convert", convert(args)) }

// The old stack (the Makefile and scripts/ at --old-prefix) names things
// differently: container wp_<name>, labels wp.*, webroot sites/<name>/wordpress
// or sites/<name>/app, database and user both wp_<name>. Everything below that
// reads "old" translates that world into a wpdock Config.
func convert(args []string) error {
	fs := flag.NewFlagSet("site-convert", flag.ContinueOnError)
	dir := fs.String("prefix", ".", "directory wpdock was installed into (default: current directory)")
	oldDir := fs.String("old-prefix", "", "root of the old stack: the directory holding its sites/ and nginx/")
	name := fs.String("name", "", "site to convert, named as the old stack knows it")
	dbHost := fs.String("db-host", "wpdock-mariadb-11", "wpdock mariadb container that receives the database")
	rootPass := fs.String("root-password", "", "mariadb root password of --db-host, which creates the database and user")
	aliases := fs.String("aliases", "", "override the aliases read back from the old vhost (comma-separated)")
	wpVer := fs.String("wp-version", "", "override the wordpress version detected from the old container")
	phpVer := fs.String("php-version", "", "override the php version detected from the old container")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	fs.Usage = ConvertUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	root, err := prefix.Resolve(*dir)
	if err != nil {
		return err
	}
	if *oldDir == "" {
		return fmt.Errorf("--old-prefix is required: the old stack's root, holding its sites/ and nginx/")
	}
	oldRoot, err := prefix.Resolve(*oldDir)
	if err != nil {
		return err
	}
	if err := (&Config{Name: *name}).checkName(); err != nil {
		return err
	}
	if *rootPass == "" {
		return fmt.Errorf("--root-password is required to create the site's database and user on %s", *dbHost)
	}

	c, oldImage, oldDB, err := inspectOldSite(oldRoot, *name)
	if err != nil {
		return err
	}
	c.DBHost = *dbHost
	if fsSet(fs, "aliases") {
		c.Aliases = *aliases
	}
	if *wpVer != "" {
		c.Version = *wpVer
	}
	if *phpVer != "" {
		c.PHP = *phpVer
	}

	old := oldContainer(*name)
	if c.PHP == "" {
		if c.PHP, err = detectPHPVersion(old); err != nil {
			return err
		}
	}
	if c.Type == "wordpress" && c.Version == "" {
		if c.Version, err = detectWPVersion(old); err != nil {
			return err
		}
	}

	if err := c.validate(); err != nil {
		return err
	}

	// The old site is left exactly as it is, so the new name and docroot both
	// have to be free — refuse rather than overwrite either.
	if siteExists(c.Name) {
		return fmt.Errorf("a site named %q already exists in wpdock — site-nuke it first if this is a retry", c.Name)
	}
	if exists(dataDir(root, c.Name)) {
		return fmt.Errorf("%s already exists — remove it first", dataDir(root, c.Name))
	}

	img, err := c.baseImage()
	if err != nil {
		return err
	}
	webroot := oldWebroot(oldRoot, c)

	fmt.Printf("convert %s\n\n", c.Name)
	field("from", fmt.Sprintf("%s (%s)", old, oldImage))
	field("to", fmt.Sprintf("%s (%s)", container(c.Name), img))
	describeCore(c)
	field("files", fmt.Sprintf("%s -> %s", webroot, dataDir(root, c.Name)))
	if c.DBName != "" {
		field("database", fmt.Sprintf("%s: %s -> %s", c.DBName, oldDB, c.DBHost))
	}
	warnConvert(c)

	if !*yes {
		ok, err := prompt.Confirm("convert it?")
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
	}

	if c.DBName != "" {
		dump, err := os.CreateTemp("", "wpdock-convert-*.sql")
		if err != nil {
			return err
		}
		dump.Close()
		defer os.Remove(dump.Name())

		if err := dumpDatabase(oldDB, c.DBName, c.DBUser, c.DBPass, dump.Name()); err != nil {
			return err
		}
		if err := db.CreateUser(c.DBHost, c.DBName, c.DBUser, c.DBPass, *rootPass); err != nil {
			return err
		}
		if err := db.ImportSQL(c.DBHost, c.DBName, c.DBUser, c.DBPass, dump.Name()); err != nil {
			return err
		}
	}

	if err := copyDocroot(root, webroot, c); err != nil {
		return err
	}

	if err := ensureNetwork(); err != nil {
		return err
	}
	if err := run(root, c, img); err != nil {
		return err
	}
	if err := writeVhost(root, c); err != nil {
		return err
	}
	// During a migration the old stack's nginx still holds ports 80/443, so
	// the wpdock proxy is usually not running yet. The vhost is on disk and
	// is read when the proxy starts, so that is a note, not a failure.
	proxyDown := false
	if err := reload(); err != nil {
		proxyDown = true
		fmt.Printf("\nnote: %v\n      the vhost is written and is picked up when the proxy starts\n", err)
	}

	fmt.Printf("\nconverted %s\n\nnext steps:\n", c.Name)
	step := 1
	if proxyDown {
		fmt.Printf("  %d. cut over: docker stop the old stack's nginx, then\n     docker compose -f %s up -d\n", step, filepath.Join(root, "docker-compose.yml"))
		step++
	}
	fmt.Printf("  %d. check the site on http://%s/ — the old copy is still what was live until the cutover\n", step, c.Domain)
	step++
	fmt.Printf("  %d. https: wpdock ssl --prefix=%s --name=%s --email=<you>\n", step, root, c.Name)
	step++
	fmt.Printf("  %d. retire the old container: docker stop %s (its files and database are untouched)\n", step, old)
	return nil
}

func oldContainer(name string) string { return "wp_" + name }

// inspectOldSite reads the old container's labels, env and limits into a
// wpdock Config, and returns the old image and database container with it.
// The versions stay empty here: they come from flags or detection.
func inspectOldSite(oldRoot, name string) (*Config, string, string, error) {
	old := oldContainer(name)

	labels := map[string]string{}
	if err := inspectContainer(old, "{{json .Config.Labels}}", &labels); err != nil {
		return nil, "", "", fmt.Errorf("no old-stack container %s: %v", old, err)
	}
	if labels["managed-by"] != "wp-stack" {
		return nil, "", "", fmt.Errorf("%s is not managed by the old stack (no managed-by=wp-stack label)", old)
	}

	var image string
	if err := inspectContainer(old, "{{json .Config.Image}}", &image); err != nil {
		return nil, "", "", err
	}
	var env []string
	if err := inspectContainer(old, "{{json .Config.Env}}", &env); err != nil {
		return nil, "", "", err
	}
	var hc struct {
		Memory    int64
		NanoCpus  int64
		PidsLimit *int64
	}
	if err := inspectContainer(old, "{{json .HostConfig}}", &hc); err != nil {
		return nil, "", "", err
	}

	c := &Config{
		Name:    name,
		Domain:  labels["wp.domain"],
		Aliases: oldAliases(oldRoot, name),
		Memory:  memoryFlag(hc.Memory),
		CPU:     cpuFlag(hc.NanoCpus),
		PIDs:    pidsFlag(hc.PidsLimit),
	}

	// The old stack's env names differ by type: WORDPRESS_DB_* for wp sites,
	// bare DB_* for php ones (which may have no database at all).
	switch labels["wp.type"] {
	case "wp":
		c.Type = "wordpress"
		c.DBName = envValue(env, "WORDPRESS_DB_NAME")
		c.DBUser = envValue(env, "WORDPRESS_DB_USER")
		c.DBPass = envValue(env, "WORDPRESS_DB_PASSWORD")
	case "php":
		c.Type = "php"
		c.DBName = envValue(env, "DB_NAME")
		c.DBUser = envValue(env, "DB_USER")
		c.DBPass = envValue(env, "DB_PASSWORD")
	default:
		return nil, "", "", fmt.Errorf("%s has wp.type=%q, want wp or php", old, labels["wp.type"])
	}

	oldDB := envValue(env, "WORDPRESS_DB_HOST")
	if oldDB == "" {
		oldDB = envValue(env, "DB_HOST")
	}
	if c.DBName != "" && oldDB == "" {
		return nil, "", "", fmt.Errorf("%s has a database but no DB host in its environment", old)
	}

	return c, image, oldDB, nil
}

// The old stack puts a wp site's files in sites/<name>/wordpress and a php
// site's in sites/<name>/app.
func oldWebroot(oldRoot string, c *Config) string {
	sub := "app"
	if c.Type == "wordpress" {
		sub = "wordpress"
	}
	return filepath.Join(oldRoot, "sites", c.Name, sub)
}

// oldAliases reads the "# Aliases:" header the old stack's templates render
// into every vhost — the same line its own scripts parse back — and turns the
// space-separated list into wpdock's comma-separated one. A missing vhost or
// header just means no aliases.
func oldAliases(oldRoot, name string) string {
	b, err := os.ReadFile(filepath.Join(oldRoot, "nginx", "conf.d", name+".conf"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "# Aliases:"); ok {
			return strings.Join(strings.Fields(rest), ",")
		}
	}
	return ""
}

func detectPHPVersion(old string) (string, error) {
	out, err := dockerOut("exec", old, "php", "-r", "echo PHP_MAJOR_VERSION.'.'.PHP_MINOR_VERSION;")
	if err != nil {
		return "", fmt.Errorf("detecting the php version: %v (is the old container running? --php-version skips detection)", err)
	}
	return strings.TrimSpace(out), nil
}

func detectWPVersion(old string) (string, error) {
	out, err := dockerOut("exec", old, "php", "-r", "include '/var/www/html/wp-includes/version.php'; echo $wp_version;")
	if err != nil {
		return "", fmt.Errorf("detecting the wordpress version: %v (is the old container running? --wp-version skips detection)", err)
	}
	return strings.TrimSpace(out), nil
}

func memoryFlag(b int64) string {
	if b <= 0 {
		return ""
	}
	const mib = 1024 * 1024
	if b%mib == 0 {
		return strconv.FormatInt(b/mib, 10) + "m"
	}
	return strconv.FormatInt(b, 10)
}

func cpuFlag(nano int64) string {
	if nano <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(nano)/1e9, 'f', -1, 64)
}

func pidsFlag(p *int64) string {
	if p == nil || *p <= 0 {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

func warnConvert(c *Config) {
	fmt.Println()
	if c.Aliases != "" {
		fmt.Println("  ! the old stack 301-redirected the aliases to the domain; wpdock serves them as the site")
	}
	if c.Type == "php" {
		fmt.Printf("  ! the old php image shipped mysqli/pdo_mysql/gd/intl/zip; php:%s-apache ships none of them\n", c.PHP)
		fmt.Println("  ! the app's DB_HOST/DB_NAME/DB_USER/DB_PASSWORD arrive renamed as WORDPRESS_DB_*")
		fmt.Println("  ! the old nginx guards (deny /vendor, .env, *.sql, ...) are gone — recreate them in an .htaccess")
	}
	fmt.Println("  ! the old site keeps running and serving; anything written to it after this copy stays behind")
	fmt.Println()
}

// dumpDatabase writes the old database to a file on the host, connecting as
// the site's own user, whose grants cover dumping its one database and
// nothing else. The dump streams to the file rather than through memory.
func dumpDatabase(container, name, user, password, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("docker", "exec", "-e", "MYSQL_PWD="+password, container,
		"mariadb-dump", "--single-transaction", "--add-drop-table", "-u"+user, name)

	var errs bytes.Buffer
	cmd.Stdout = f
	cmd.Stderr = &errs

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errs.String()); msg != "" {
			return fmt.Errorf("dumping %s from %s: %v: %s", name, container, err, msg)
		}
		return fmt.Errorf("dumping %s from %s: %v", name, container, err)
	}
	return nil
}

// copyDocroot copies the old webroot into <prefix>/data/<name> inside a root
// helper container, since the files are owned by the old container's users.
// Ownership moves from Alpine's www-data (82) to Debian's (33), which is who
// runs in the wordpress/php apache images. A wp site also gets the standard
// WordPress .htaccess if it has none: the old stack's nginx did the rewriting,
// so the files won't have one, and without it Apache 404s pretty permalinks.
func copyDocroot(root, webroot string, c *Config) error {
	var sh strings.Builder
	fmt.Fprintf(&sh, "set -e\nmkdir -p /data/%[1]s\ncp -a /src/. /data/%[1]s/\n", c.Name)
	if c.Type == "wordpress" {
		fmt.Fprintf(&sh, `if [ ! -f /data/%[1]s/.htaccess ]; then
cat > /data/%[1]s/.htaccess <<'EOF'
# BEGIN WordPress
<IfModule mod_rewrite.c>
RewriteEngine On
RewriteBase /
RewriteRule ^index\.php$ - [L]
RewriteCond %%{REQUEST_FILENAME} !-f
RewriteCond %%{REQUEST_FILENAME} !-d
RewriteRule . /index.php [L]
</IfModule>
# END WordPress
EOF
fi
`, c.Name)
	}
	fmt.Fprintf(&sh, "chown -R 33:33 /data/%s\n", c.Name)

	return helper(root, []string{"-v", webroot + ":/src:ro"}, sh.String())
}

func fsSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}
