package site

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"wpdock/src/prefix"
)

func SSLUsage() {
	fmt.Fprint(os.Stderr, `  ssl --prefix=<path> --name=<site> --email=<address> [--staging]
        get a Let's Encrypt certificate for the site and switch it to https
  ssl --prefix=<path> --renew
        renew every certificate that is due and reload nginx
`)
}

func SSL(args []string) error { return fail("ssl", ssl(args)) }

func ssl(args []string) error {
	fs := flag.NewFlagSet("ssl", flag.ContinueOnError)
	dir := fs.String("prefix", ".", "directory wpdock was installed into (default: current directory)")
	name := fs.String("name", "", "site to get a certificate for")
	email := fs.String("email", "", "address Let's Encrypt registers and sends expiry notices to")
	staging := fs.Bool("staging", false, "use the Let's Encrypt staging environment (untrusted test certificates)")
	renew := fs.Bool("renew", false, "renew due certificates instead of issuing one")
	fs.Usage = SSLUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	root, err := prefix.Resolve(*dir)
	if err != nil {
		return err
	}

	if *renew {
		if err := certbot(root, "renew", "--webroot", "-w", "/webroot", "--non-interactive"); err != nil {
			return err
		}
		return reload()
	}

	if err := (&Config{Name: *name}).checkName(); err != nil {
		return err
	}
	if *email == "" {
		return fmt.Errorf("--email is required: Let's Encrypt registers it and sends expiry notices there")
	}

	c, err := inspect(*name)
	if err != nil {
		return err
	}

	if !exists(filepath.Join(root, "nginx", "templates", sslTemplate)) {
		return fmt.Errorf("%s is missing — rerun `wpdock install --prefix=%s` to add it", sslTemplate, root)
	}
	if err := ensureProxyPort("443", root); err != nil {
		return err
	}

	// Rewrite the vhost from the current template first, so nginx serves the
	// acme-challenge location before certbot asks for the domain to be proved.
	if err := publish(root, c); err != nil {
		return err
	}

	certArgs := []string{
		"certonly", "--webroot", "-w", "/webroot",
		"--cert-name", c.Domain,
		"--email", *email, "--agree-tos", "--non-interactive",
	}
	for _, d := range strings.Fields(serverName(c)) {
		certArgs = append(certArgs, "-d", d)
	}
	if *staging {
		certArgs = append(certArgs, "--staging")
	}
	if err := certbot(root, certArgs...); err != nil {
		return err
	}

	// The certificate exists now, so publish renders the https vhost.
	if err := publish(root, c); err != nil {
		return err
	}

	fmt.Printf("\nssl on for %s\n\n", c.Name)
	field("domains", serverName(c))
	field("certificate", certLive(root, c.Domain))
	fmt.Printf("\nhttp now redirects to https. renew with: wpdock ssl --renew --prefix=%s\n", root)
	return nil
}

// certbot runs the certbot container against the install's certs/ and www/
// directories, streaming its output, since what certbot prints — the expiry
// date, or why a challenge failed — is the result of the command.
func certbot(root string, args ...string) error {
	run := []string{
		"run", "--rm",
		"-v", filepath.Join(root, "certs") + ":/etc/letsencrypt",
		"-v", filepath.Join(root, "www") + ":/webroot",
		certbotImage,
	}

	cmd := exec.Command("docker", append(run, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("certbot: %v", err)
	}
	return nil
}

// ensureProxyPort refuses to set up https that nothing could reach: the
// compose file has to publish the port before a vhost can listen on it.
func ensureProxyPort(port, root string) error {
	out, err := dockerOut("ps", "--filter", "label="+proxyLabel, "--format", "{{.Ports}}")
	if err != nil {
		return err
	}
	if strings.Contains(out, ":"+port+"->") {
		return nil
	}
	return fmt.Errorf("the proxy does not publish port %s — rerun `wpdock install --prefix=%s --force` to regenerate docker-compose.yml, then `docker compose -f %s up -d`",
		port, root, filepath.Join(root, "docker-compose.yml"))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
