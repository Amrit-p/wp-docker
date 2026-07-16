package site

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
)

func requireWordPress(name string) (*Config, error) {
	c, err := inspect(name)
	if err != nil {
		return nil, err
	}
	if c.Type != "wordpress" {
		return nil, fmt.Errorf("%s is a %s site, not wordpress", name, c.Type)
	}
	return c, nil
}

func wpExec(name string, env []string, php string) (string, error) {
	args := []string{"exec"}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, container(name), "php", "-r", php)
	return dockerOut(args...)
}

const wpListPHP = `error_reporting(0);
require "/var/www/html/wp-load.php";
global $wpdb;
$rows = $wpdb->get_results("SELECT ID, user_login, user_email, user_registered, display_name FROM {$wpdb->users} ORDER BY ID", ARRAY_N);
foreach ($rows as $r) { echo implode("\t", $r), "\n"; }`

const wpResetPHP = `error_reporting(0);
require "/var/www/html/wp-load.php";
$id = (int) getenv("WP_UID");
$login = getenv("WP_LOGIN");
if ($id) {
	$u = get_user_by("id", $id);
	if (!$u) { fwrite(STDERR, "no user with ID " . $id); exit(1); }
} elseif ($login) {
	$u = get_user_by("login", $login);
	if (!$u) { fwrite(STDERR, "no user with login " . $login); exit(1); }
} else {
	$us = get_users(array("role" => "administrator", "orderby" => "ID", "order" => "ASC", "number" => 1));
	if (!$us) { fwrite(STDERR, "no administrator found"); exit(1); }
	$u = $us[0];
}
wp_set_password(getenv("WP_PW"), $u->ID);
echo $u->ID, "\t", $u->user_login;`

func WPListUsersUsage() {
	fmt.Fprint(os.Stderr, `  site-wp-list-users [--prefix=<path>] --name=<site>
        list the WordPress users (wp_users) of a site
`)
}

func WPListUsers(args []string) error { return fail("site-wp-list-users", wpListUsers(args)) }

func wpListUsers(args []string) error {
	fs := flag.NewFlagSet("site-wp-list-users", flag.ContinueOnError)
	name := fs.String("name", "", "site name")
	fs.String("prefix", "", "installation directory (accepted for consistency; not used)")
	fs.Usage = WPListUsersUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	if err := (&Config{Name: *name}).checkName(); err != nil {
		return err
	}
	c, err := requireWordPress(*name)
	if err != nil {
		return err
	}

	out, err := wpExec(*name, dbEnvPairs(c), wpListPHP)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLOGIN\tEMAIL\tREGISTERED\tNAME")
	if out = strings.TrimRight(out, "\n"); out != "" {
		for _, line := range strings.Split(out, "\n") {
			fmt.Fprintln(w, line)
		}
	}
	return w.Flush()
}

func WPResetPasswordUsage() {
	fmt.Fprint(os.Stderr, `  site-wp-reset-password [--prefix=<path>] --name=<site> [--userID=<id> | --user=<login>] --password=<pass>
        set a WordPress user's password (default: the first administrator)
`)
}

func WPResetPassword(args []string) error {
	return fail("site-wp-reset-password", wpResetPassword(args))
}

func wpResetPassword(args []string) error {
	fs := flag.NewFlagSet("site-wp-reset-password", flag.ContinueOnError)
	name := fs.String("name", "", "site name")
	userID := fs.String("userID", "", "the wp_users ID to reset (default: the first administrator)")
	user := fs.String("user", "", "the user_login to reset, instead of --userID")
	password := fs.String("password", "", "the new password")
	fs.String("prefix", "", "installation directory (accepted for consistency; not used)")
	fs.Usage = WPResetPasswordUsage
	if ok, err := parse(fs, args); !ok {
		return err
	}

	if err := (&Config{Name: *name}).checkName(); err != nil {
		return err
	}
	if *userID != "" && *user != "" {
		return fmt.Errorf("--userID and --user name the same thing, so pass one of them")
	}
	if *userID != "" {
		if _, err := strconv.Atoi(*userID); err != nil {
			return fmt.Errorf("--userID must be a number")
		}
	}
	if *password == "" {
		return fmt.Errorf("--password is required")
	}
	c, err := requireWordPress(*name)
	if err != nil {
		return err
	}

	env := append(dbEnvPairs(c), "WP_UID="+*userID, "WP_LOGIN="+*user, "WP_PW="+*password)
	out, err := wpExec(*name, env, wpResetPHP)
	if err != nil {
		return err
	}

	id, login, _ := strings.Cut(strings.TrimSpace(out), "\t")
	fmt.Printf("reset password for user %s (ID %s) on %s\n", login, id, *name)
	return nil
}
