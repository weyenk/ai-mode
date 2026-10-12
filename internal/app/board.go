package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The work board is a Vikunja server that runs on its own (24/7 under launchd, Postgres behind it); ai-mode only
// talks to its API. `ai-mode board init` signs in once as the human who owns the board, creates one bot user per
// agent (roadmap "Card notes": a note is written by its stage's bot, so the board shows who wrote it) and stores an
// API token per bot plus one for ai-mode in ~/.config/ai-mode/board.json (0600). The password is never stored.

const boardDefaultURL = "http://127.0.0.1:3456"

// boardBots are the agents that write on the board (work-board spec, "Identity").
var boardBots = []string{"bot-chief", "bot-product", "bot-architect", "bot-amigos", "bot-qa", "bot-coder"}

type boardBot struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

type boardCreds struct {
	URL   string              `json:"url"`
	Owner string              `json:"owner"`       // the human account that owns the bots and the projects
	Token string              `json:"owner_token"` // ai-mode's API token on the owner's account
	Bots  map[string]boardBot `json:"bots"`
}

func boardCredsPath() string { return filepath.Join(configDir(), "board.json") }

func boardURL() string {
	if u := firstNonEmpty(os.Getenv("AI_MODE_BOARD_URL"), configValue("BOARD_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	if c, err := loadBoardCreds(); err == nil && c.URL != "" {
		return c.URL
	}
	return boardDefaultURL
}

func loadBoardCreds() (boardCreds, error) {
	var c boardCreds
	data, err := os.ReadFile(boardCredsPath())
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%s: %w", boardCredsPath(), err)
	}
	if c.Token == "" {
		return c, fmt.Errorf("%s has no owner token; run ai-mode board init", boardCredsPath())
	}
	return c, nil
}

func saveBoardCreds(c boardCreds) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	tmp := boardCredsPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, boardCredsPath())
}

// pickPermissions keeps the wanted actions the server offers (its /routes list), so a token never asks for an
// action this Vikunja version does not have.
func pickPermissions(avail map[string]map[string]json.RawMessage, wanted map[string][]string) map[string][]string {
	out := map[string][]string{}
	for group, actions := range wanted {
		have, ok := avail[group]
		if !ok {
			continue
		}
		var picked []string
		for _, a := range actions {
			if a == "*" {
				for k := range have {
					picked = append(picked, k)
				}
				continue
			}
			if _, ok := have[a]; ok {
				picked = append(picked, a)
			}
		}
		sort.Strings(picked)
		if len(picked) > 0 {
			out[group] = picked
		}
	}
	return out
}

// botPermissions: read the board, create and update tasks, move them between buckets, attach labels, comment and
// relate. No deletes, no bots, no tokens.
var botPermissions = map[string][]string{
	"tasks": {"read_one", "read_all", "create", "update", "position", "read"},
	"projects": {"read_one", "read_all", "tasks_by-index", "views_buckets", "views_buckets_tasks",
		"views_buckets_tasks_put", "views_tasks"},
	"projects_views":  {"read_one", "read_all"},
	"labels":          {"read_one", "read_all", "create"},
	"tasks_comments":  {"read_one", "read_all", "create"},
	"tasks_relations": {"create"},
	"tasks_labels":    {"read_all", "create", "delete"},
}

// ownerPermissions: ai-mode's token on the owner's account also creates the project, its buckets and labels and
// shares the project with the bots.
var ownerPermissions = map[string][]string{
	"projects": {"*"}, "projects_views": {"*"}, "projects_users": {"*"}, "labels": {"*"}, "tasks": {"*"},
	"tasks_comments": {"*"}, "tasks_relations": {"*"}, "tasks_labels": {"*"}, "user": {"read", "read_one"},
}

const boardUsage = `usage: ai-mode board <command>
  init [--url URL] [--register]   sign in once as the board's owner (prompts in the terminal), create the bots
                                  and store their tokens in ~/.config/ai-mode/board.json (0600)
  status                          the server, its version, and whether the stored tokens work
  sync [--repo DIR]               pull what humans did on the board (new requests in init, cards moved to done)
                                  and push the repo's cards, columns, labels and notes`

func cmdBoard(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, boardUsage)
		return 2
	}
	sub := args[0]
	fs := newFlags("board " + sub)
	urlF := fs.String("url", "", "init: the Vikunja server (default "+boardDefaultURL+")")
	register := fs.Bool("register", false, "init: create the owner account first")
	repo := fs.String("repo", ".", "sync: the target Go module whose board to sync")
	_, code, ok := parse(fs, args[1:])
	if !ok {
		return code
	}
	switch sub {
	case "init":
		base := boardURL()
		if *urlF != "" {
			base = strings.TrimRight(*urlF, "/")
		}
		return boardInit(base, *register)
	case "status":
		problems := boardStatus(os.Stdout)
		if len(problems) > 0 {
			for _, p := range problems {
				fmt.Println("  - " + p)
			}
			return 1
		}
		return 0
	case "sync":
		pl, err := newPipeline(*repo, true, false)
		if err != nil {
			return fail("%v", err)
		}
		if err := pl.attachBoard(); err != nil {
			return fail("%v", err)
		}
		if pl.vk == nil {
			return fail("no board: run ai-mode board init (or unset AI_MODE_BOARD=off)")
		}
		release, err := acquireLock(pipelineLockPath(pl.target))
		if err != nil {
			return fail("%v", err)
		}
		defer release()
		if err := pl.vk.pull(); err != nil {
			return fail("%v", err)
		}
		pl.sync("pipeline: board sync")
		fmt.Printf("%s/projects/%d/%d\n", pl.vk.creds.URL, pl.vk.st.Project, pl.vk.st.View)
		return 0
	}
	fmt.Fprintln(os.Stderr, boardUsage)
	return 2
}

// readTTY prompts on the terminal; hidden turns off echo for a password.
func readTTY(prompt string, hidden bool) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", errors.New("board init needs a terminal: run it yourself in a shell")
	}
	defer tty.Close()
	fmt.Fprint(tty, prompt)
	if hidden {
		stty := func(arg string) {
			c := exec.Command("stty", arg)
			c.Stdin = tty
			_ = c.Run()
		}
		stty("-echo")
		defer func() { stty("echo"); fmt.Fprintln(tty) }()
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	return strings.TrimSpace(line), err
}

func boardInit(base string, register bool) int {
	if _, err := vkInfo(base); err != nil {
		return fail("no Vikunja at %s: %v", base, err)
	}
	user, err := readTTY("Vikunja username (the board's human owner): ", false)
	if err != nil || user == "" {
		return fail("a username is required: %v", err)
	}
	if strings.HasPrefix(user, "bot-") {
		return fail("the owner is a human account; bot- names are for agents")
	}
	pass, err := readTTY("Password: ", true)
	if err != nil || pass == "" {
		return fail("a password is required")
	}
	anon := newVK(base, "")
	if register {
		email, err := readTTY("Email (for the account; Vikunja does not send mail here): ", false)
		if err != nil || email == "" {
			return fail("an email is required to register")
		}
		if err := anon.register(user, email, pass); err != nil {
			return fail("register: %v", err)
		}
		fmt.Printf("Registered %s.\n", user)
	}
	jwt, err := anon.login(user, pass)
	pass = ""
	if err != nil {
		if vkStatus(err) == 401 || vkStatus(err) == 412 || vkStatus(err) == 404 {
			return fail("sign-in failed for %s (no such account? create it at %s or re-run with --register): %v", user, base, err)
		}
		return fail("sign-in: %v", err)
	}
	session := newVK(base, jwt)
	routes, err := session.routes()
	if err != nil {
		return fail("token routes: %v", err)
	}
	creds, _ := loadBoardCreds()
	if creds.Owner != "" && creds.Owner != user {
		fmt.Printf("Replacing the board owner %s with %s.\n", creds.Owner, user)
		creds = boardCreds{}
	}
	creds.URL, creds.Owner = base, user
	if creds.Bots == nil {
		creds.Bots = map[string]boardBot{}
	}
	owner, err := session.createToken(vkToken{Title: "ai-mode", Permissions: pickPermissions(routes, ownerPermissions), ExpiresAt: vkFarFuture})
	if err != nil || owner.Token == "" {
		return fail("owner token: %v", err)
	}
	creds.Token = owner.Token
	existing, err := session.bots()
	if err != nil {
		return fail("list bots: %v", err)
	}
	byName := map[string]vkUser{}
	for _, b := range existing {
		byName[b.Username] = b
	}
	perms := pickPermissions(routes, botPermissions)
	for _, name := range boardBots {
		bot, ok := byName[name]
		if !ok {
			if bot, err = session.createBot(name, strings.TrimPrefix(name, "bot-")+" (ai-mode)"); err != nil {
				return fail("create %s: %v (bot usernames are global; delete a stale one in Settings → Bots)", name, err)
			}
			fmt.Printf("Created bot user %s.\n", name)
		}
		tok, err := session.createToken(vkToken{Title: "ai-mode " + name, Permissions: perms, ExpiresAt: vkFarFuture, OwnerID: bot.ID})
		if err != nil || tok.Token == "" {
			return fail("token for %s: %v", name, err)
		}
		creds.Bots[name] = boardBot{ID: bot.ID, Token: tok.Token}
	}
	if err := saveBoardCreds(creds); err != nil {
		return fail("%v", err)
	}
	fmt.Printf("Board ready at %s: owner %s, %d bots. Tokens are in %s (0600).\n", base, user, len(creds.Bots), boardCredsPath())
	fmt.Println("Next: ai-mode board sync --repo <target>   (the pipeline also syncs after every stage)")
	return 0
}

// boardStatus prints the board's state and returns the problems doctor reports.
func boardStatus(w *os.File) []string {
	base := boardURL()
	var problems []string
	info, err := vkInfo(base)
	if err != nil {
		fmt.Fprintf(w, "work board:   %s  NOT REACHABLE\n", base)
		return append(problems, fmt.Sprintf("Vikunja is not answering at %s (%v). It runs outside ai-mode: launchctl kickstart -k gui/$(id -u)/local.vikunja, log in ~/.local/share/vikunja/logs/vikunja.log", base, err))
	}
	fmt.Fprintf(w, "work board:   %s  Vikunja %v  OK\n", base, info["version"])
	creds, err := loadBoardCreds()
	if errors.Is(err, os.ErrNotExist) {
		return append(problems, "No board tokens: run `ai-mode board init` in a terminal (signs in once as the board's owner, creates the bots)")
	}
	if err != nil {
		return append(problems, err.Error())
	}
	if err := newVK(base, creds.Token).do("GET", "/token/test", nil, nil); err != nil {
		problems = append(problems, fmt.Sprintf("ai-mode's owner token is refused (%v): re-run ai-mode board init", err))
	} else {
		fmt.Fprintf(w, "board owner:  %s\n", creds.Owner)
	}
	var ok, bad []string
	for _, name := range boardBots {
		b, have := creds.Bots[name]
		if !have {
			bad = append(bad, name+" (no token)")
			continue
		}
		if err := newVK(base, b.Token).do("GET", "/token/test", nil, nil); err != nil {
			bad = append(bad, name+" (token refused)")
			continue
		}
		ok = append(ok, name)
	}
	fmt.Fprintf(w, "board bots:   %d/%d tokens work\n", len(ok), len(boardBots))
	if len(bad) > 0 {
		problems = append(problems, "Board bots without a working token: "+strings.Join(bad, ", ")+"; re-run ai-mode board init")
	}
	return problems
}
