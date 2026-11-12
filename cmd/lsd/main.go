package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LewdLinux1337/luscious-downloader/internal/downloader"
	"github.com/LewdLinux1337/luscious-downloader/internal/luscious"
	"github.com/LewdLinux1337/luscious-downloader/internal/media"
	"github.com/LewdLinux1337/luscious-downloader/internal/netx"
)

type options struct {
	albumInput     string
	userInput      string
	keyword        string
	readList       bool
	onlyFavorites  bool
	groupByUser    bool
	searchDownload bool
	menu           bool
	page           int
	maxPage        int
	sorting        string
	output         string
	threads        int
	retries        int
	timeout        int
	delay          int
	nameFormat     string
	cookieFile     string
	sessionCookie  string
	makePDF        bool
	makeCBZ        bool
	removeOrigin   bool
}

type settings struct {
	Directory        string `json:"directory"`
	Pool             int    `json:"pool"`
	Retries          int    `json:"retries"`
	Timeout          int    `json:"timeout"`
	Delay            int    `json:"delay"`
	FoldernameFormat string `json:"foldername_format"`
	GenPDF           bool   `json:"gen_pdf"`
	GenCBZ           bool   `json:"gen_cbz"`
	RemoveOrigin     bool   `json:"rm_origin_dir"`
	GroupByUser      bool   `json:"group_by_user"`
}

type app struct {
	client     *luscious.Client
	downloader *downloader.Downloader
	workers    int
}

const maxWorkers = 128

var invalidFolderChars = regexp.MustCompile(`[\\/:*?"<>|]`)
var invalidTitleChars = regexp.MustCompile(`[^\p{L}\p{N}_ .-]`)

func main() {
	log.SetFlags(log.Ltime)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		log.Printf("ERROR: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	opt, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	opt.sessionCookie, err = readCookieFile(opt.cookieFile)
	if err != nil {
		return err
	}
	if opt.menu {
		return runMenu(ctx, opt.sessionCookie)
	}
	return execute(ctx, opt)
}

func parseOptions(args []string) (options, error) {
	opt := options{page: 1, maxPage: 1, sorting: "date_trending", threads: defaultWorkers(), retries: 5, timeout: 30, nameFormat: "[%i]%t"}
	flags := flag.NewFlagSet("lsd", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.StringVar(&opt.albumInput, "album", "", "download album by URL or ID")
	flags.StringVar(&opt.albumInput, "a", "", "download album by URL or ID")
	flags.StringVar(&opt.userInput, "user", "", "download a user's albums by URL or ID")
	flags.StringVar(&opt.userInput, "u", "", "download a user's albums by URL or ID")
	flags.StringVar(&opt.keyword, "search", "", "search albums by keyword")
	flags.StringVar(&opt.keyword, "s", "", "search albums by keyword")
	flags.BoolVar(&opt.readList, "list", false, "download albums listed in ./list.txt")
	flags.BoolVar(&opt.readList, "l", false, "download albums listed in ./list.txt")
	flags.BoolVar(&opt.onlyFavorites, "favorites", false, "download only a user's favorite albums")
	flags.BoolVar(&opt.onlyFavorites, "f", false, "download only a user's favorite albums")
	flags.BoolVar(&opt.groupByUser, "group", false, "group albums under the uploader's folder")
	flags.BoolVar(&opt.groupByUser, "agroup", false, "group albums under the uploader's folder")
	flags.BoolVar(&opt.groupByUser, "g", false, "group albums under the uploader's folder")
	flags.BoolVar(&opt.searchDownload, "download", false, "download albums from search results")
	flags.BoolVar(&opt.searchDownload, "d", false, "download albums from search results")
	flags.IntVar(&opt.page, "page", 1, "starting search page")
	flags.IntVar(&opt.maxPage, "max-page", 1, "last search page")
	flags.StringVar(&opt.sorting, "sorting", "date_trending", "search sort: date_trending or rating_all_time")
	flags.BoolVar(&opt.makePDF, "pdf", false, "create a PDF for each downloaded album")
	flags.BoolVar(&opt.makePDF, "p", false, "create a PDF for each downloaded album")
	flags.BoolVar(&opt.makeCBZ, "cbz", false, "create a CBZ for each downloaded album")
	flags.BoolVar(&opt.makeCBZ, "c", false, "create a CBZ for each downloaded album")
	flags.BoolVar(&opt.removeOrigin, "rm-origin-dir", false, "remove album folder after successful conversion")
	flags.StringVar(&opt.output, "output", "", "output directory (default: current directory)")
	flags.StringVar(&opt.output, "o", "", "output directory (default: current directory)")
	flags.IntVar(&opt.threads, "threads", defaultWorkers(), "maximum concurrent network workers")
	flags.IntVar(&opt.threads, "t", defaultWorkers(), "maximum concurrent network workers")
	flags.IntVar(&opt.retries, "retries", 5, "additional media download retries")
	flags.IntVar(&opt.retries, "R", 5, "additional media download retries")
	flags.IntVar(&opt.timeout, "timeout", 30, "HTTP timeout in seconds")
	flags.IntVar(&opt.timeout, "T", 30, "HTTP timeout in seconds")
	flags.IntVar(&opt.delay, "delay", 0, "delay between album downloads in seconds")
	flags.IntVar(&opt.delay, "D", 0, "delay between album downloads in seconds")
	flags.StringVar(&opt.nameFormat, "format", "[%i]%t", "album folder format: %i ID, %t title, %a author, %p pictures, %g GIFs")
	flags.StringVar(&opt.cookieFile, "cookie-file", os.Getenv("LUSCIOUS_COOKIE_FILE"), "read first-party session Cookie header from file")
	flags.BoolVar(&opt.menu, "menu", false, "open interactive menu")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Luscious Downloader")
		fmt.Fprintln(flags.Output(), "Usage: lsd [--album URL_OR_ID | --user URL_OR_ID | --search WORD | --list] [options]")
		fmt.Fprintln(flags.Output(), "Run lsd without arguments or with --menu to open the interactive menu.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return opt, err
	}
	opt.keyword = strings.TrimSpace(opt.keyword)
	if len(flags.Args()) > 0 {
		return opt, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	actions := 0
	for _, selected := range []bool{opt.albumInput != "", opt.userInput != "", opt.keyword != "", opt.readList} {
		if selected {
			actions++
		}
	}
	if opt.menu && actions > 0 {
		return opt, fmt.Errorf("--menu cannot be combined with download options")
	}
	if actions == 0 {
		if len(args) == 0 {
			opt.menu = true
		} else if !opt.menu {
			return opt, fmt.Errorf("choose one of --album, --user, --search, or --list")
		}
	} else if actions > 1 {
		return opt, fmt.Errorf("choose one of --album, --user, --search, or --list")
	}
	if opt.onlyFavorites && opt.userInput == "" {
		return opt, fmt.Errorf("--favorites requires --user")
	}
	if opt.searchDownload && opt.keyword == "" {
		return opt, fmt.Errorf("--download requires --search")
	}
	if opt.sorting != "date_trending" && opt.sorting != "rating_all_time" {
		return opt, fmt.Errorf("unsupported sorting value %q", opt.sorting)
	}
	if opt.page < 1 {
		opt.page = 1
	}
	if opt.maxPage < opt.page {
		opt.maxPage = opt.page
	}
	if opt.threads < 1 {
		opt.threads = defaultWorkers()
	}
	if opt.threads > maxWorkers {
		log.Printf("WARNING: --threads capped at %d", maxWorkers)
		opt.threads = maxWorkers
	}
	if opt.retries < 0 {
		opt.retries = 0
	}
	if opt.timeout < 1 {
		return opt, fmt.Errorf("--timeout must be greater than zero")
	}
	if opt.delay < 0 {
		opt.delay = 0
	}
	if opt.output == "" {
		opt.output = "."
	}
	opt.output, _ = filepath.Abs(opt.output)
	if opt.removeOrigin && !opt.makePDF && !opt.makeCBZ {
		log.Print("WARNING: --rm-origin-dir ignored without --pdf or --cbz")
		opt.removeOrigin = false
	}
	if opt.keyword != "" && !opt.searchDownload && (opt.makePDF || opt.makeCBZ || opt.removeOrigin) {
		log.Print("WARNING: conversion options ignored unless --download is set")
		opt.makePDF, opt.makeCBZ, opt.removeOrigin = false, false, false
	}
	return opt, nil
}

func readCookieFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read cookie file: %w", err)
	}
	cookie := strings.TrimSpace(string(data))
	if strings.HasPrefix(strings.ToLower(cookie), "cookie:") {
		cookie = strings.TrimSpace(cookie[len("Cookie:"):])
	}
	if len(cookie) > 16<<10 || strings.ContainsAny(cookie, "\r\n") {
		return "", errors.New("cookie file must contain one Cookie header value under 16 KiB")
	}
	header := http.Header{}
	header.Set("Cookie", cookie)
	if len((&http.Request{Header: header}).Cookies()) == 0 {
		return "", errors.New("cookie file must contain a valid Cookie header value")
	}
	return cookie, nil
}

func execute(ctx context.Context, opt options) error {
	tool := downloadOptions(opt)
	switch {
	case opt.albumInput != "":
		ids, parseErr := parseIDs(opt.albumInput, "album")
		_, runErr := tool.downloadAlbums(ctx, ids, opt, opt.output)
		return errors.Join(parseErr, runErr)
	case opt.userInput != "":
		ids, parseErr := parseIDs(opt.userInput, "user")
		_, runErr := tool.downloadUsers(ctx, ids, opt)
		return errors.Join(parseErr, runErr)
	case opt.keyword != "":
		return tool.search(ctx, opt)
	case opt.readList:
		return tool.downloadList(ctx, opt, false)
	default:
		return nil
	}
}

func parseIDs(input, kind string) ([]string, error) {
	seen := make(map[string]bool)
	var ids []string
	var failures []error
	for _, value := range strings.Split(input, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := extractID(value, kind)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 && len(failures) == 0 {
		failures = append(failures, fmt.Errorf("no %s IDs provided", kind))
	}
	return ids, errors.Join(failures...)
}

func extractID(value, kind string) (string, error) {
	value = strings.TrimSpace(value)
	prefix := kind + "-"
	if strings.HasPrefix(strings.ToLower(value), prefix) {
		value = value[len(prefix):]
	}
	if id, err := positiveID(value); err == nil {
		return id, nil
	}
	parsed, err := urlParse(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s URL or ID %q", kind, value)
	}
	segments := strings.Split(strings.Trim(parsed, "/"), "/")
	if len(segments) == 0 {
		return "", fmt.Errorf("invalid %s URL %q", kind, value)
	}
	last := segments[len(segments)-1]
	if kind == "album" && !strings.Contains(strings.ToLower(parsed), "/albums/") && len(segments) > 1 {
		return "", fmt.Errorf("URL %q is not an album URL", value)
	}
	if kind == "user" && !strings.Contains(strings.ToLower(parsed), "/users/") && len(segments) > 1 {
		return "", fmt.Errorf("URL %q is not a user URL", value)
	}
	if index := strings.LastIndex(last, "_"); index >= 0 {
		last = last[index+1:]
	}
	id, err := positiveID(last)
	if err != nil {
		return "", fmt.Errorf("could not extract %s ID from %q", kind, value)
	}
	return id, nil
}

func urlParse(value string) (string, error) {
	value = strings.TrimSuffix(value, "?")
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", err
		}
		return parsed.EscapedPath(), nil
	}
	return strings.SplitN(value, "?", 2)[0], nil
}

func positiveID(value string) (string, error) {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid ID")
	}
	return strconv.FormatUint(n, 10), nil
}

func (a app) downloadAlbum(ctx context.Context, id string, opt options, output string, workers int) error {
	client := a.client.WithWorkers(workers)
	mediaDownloader := a.downloader.WithWorkers(workers)
	album, err := client.Album(ctx, id)
	if err != nil {
		return fmt.Errorf("album %s: %w", id, err)
	}
	name := albumFolderName(album, opt.nameFormat)
	directory := filepath.Join(output, name)
	log.Printf("Album %s: %s by %s", album.ID, album.Title, album.Author)
	pictures, err := client.AlbumPictures(ctx, id)
	if err != nil {
		return fmt.Errorf("album %s picture list: %w", id, err)
	}
	files := make([]downloader.File, 0, len(pictures))
	for _, picture := range pictures {
		files = append(files, downloader.File{ID: picture.ID, URL: picture.URL})
	}
	downloadErr := mediaDownloader.Download(ctx, files, directory)
	if downloadErr != nil {
		log.Printf("ERROR: album %s media download: %v", id, downloadErr)
	}
	conversionOK := true
	var conversionErrors []error
	if opt.makePDF {
		if err := media.CreatePDF(filepath.Join(output, name+".pdf"), directory); err != nil {
			conversionErrors = append(conversionErrors, fmt.Errorf("album %s PDF: %w", id, err))
			conversionOK = false
		} else {
			log.Printf("PDF saved: %s", filepath.Join(output, name+".pdf"))
		}
	}
	if opt.makeCBZ {
		if err := media.CreateCBZ(filepath.Join(output, name+".cbz"), directory); err != nil {
			conversionErrors = append(conversionErrors, fmt.Errorf("album %s CBZ: %w", id, err))
			conversionOK = false
		} else {
			log.Printf("CBZ saved: %s", filepath.Join(output, name+".cbz"))
		}
	}
	if opt.removeOrigin && conversionOK && (opt.makePDF || opt.makeCBZ) && downloadErr == nil {
		if err := os.RemoveAll(directory); err != nil {
			conversionErrors = append(conversionErrors, fmt.Errorf("remove album folder %s: %w", id, err))
		}
	}
	return errors.Join(downloadErr, errors.Join(conversionErrors...))
}

func (a app) downloadAlbums(ctx context.Context, ids []string, opt options, output string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	workers := max(1, a.workers)
	if opt.delay > 0 {
		var completed []string
		var failures []error
		for index, id := range ids {
			if err := ctx.Err(); err != nil {
				failures = append(failures, err)
				break
			}
			if err := a.downloadAlbum(ctx, id, opt, output, workers); err != nil {
				failures = append(failures, err)
			} else {
				completed = append(completed, id)
			}
			if index+1 < len(ids) {
				if err := pause(ctx, time.Duration(opt.delay)*time.Second); err != nil {
					failures = append(failures, err)
					break
				}
			}
		}
		return completed, errors.Join(failures...)
	}

	albumWorkers := min(workers, len(ids))
	workersPerAlbum := (workers + albumWorkers - 1) / albumWorkers
	jobs := make(chan int)
	errs := make([]error, len(ids))
	started := make([]bool, len(ids))
	var wg sync.WaitGroup
	for range albumWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				errs[index] = a.downloadAlbum(ctx, ids[index], opt, output, workersPerAlbum)
			}
		}()
	}
	for index := range ids {
		if ctx.Err() != nil {
			break
		}
		jobs <- index
		started[index] = true
	}
	close(jobs)
	wg.Wait()
	var completed []string
	var failures []error
	for index, id := range ids {
		if !started[index] {
			continue
		}
		if errs[index] != nil {
			failures = append(failures, errs[index])
		} else {
			completed = append(completed, id)
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, err)
	}
	return completed, errors.Join(failures...)
}

func (a app) downloadUsers(ctx context.Context, ids []string, opt options) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	workers := max(1, a.workers)
	if opt.delay > 0 {
		var completed []string
		var failures []error
		for index, id := range ids {
			if err := ctx.Err(); err != nil {
				failures = append(failures, err)
				break
			}
			if err := a.downloadUser(ctx, id, opt, workers); err != nil {
				failures = append(failures, err)
			} else {
				completed = append(completed, id)
			}
			if index+1 < len(ids) {
				if err := pause(ctx, time.Duration(opt.delay)*time.Second); err != nil {
					failures = append(failures, err)
					break
				}
			}
		}
		return completed, errors.Join(failures...)
	}

	userWorkers := min(workers, len(ids))
	workersPerUser := (workers + userWorkers - 1) / userWorkers
	jobs := make(chan int)
	errs := make([]error, len(ids))
	started := make([]bool, len(ids))
	var wg sync.WaitGroup
	for range userWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				errs[index] = a.downloadUser(ctx, ids[index], opt, workersPerUser)
			}
		}()
	}
	for index := range ids {
		if ctx.Err() != nil {
			break
		}
		jobs <- index
		started[index] = true
	}
	close(jobs)
	wg.Wait()
	var completed []string
	var failures []error
	for index, id := range ids {
		if !started[index] {
			continue
		}
		if errs[index] != nil {
			failures = append(failures, errs[index])
		} else {
			completed = append(completed, id)
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, err)
	}
	return completed, errors.Join(failures...)
}

func (a app) downloadUser(ctx context.Context, id string, opt options, workers int) error {
	client := a.client.WithWorkers(workers)
	user, err := client.User(ctx, id)
	if err != nil {
		return fmt.Errorf("user %s: %w", id, err)
	}
	albumIDs, err := client.UserAlbums(ctx, id, opt.onlyFavorites)
	if err != nil {
		return fmt.Errorf("user %s albums: %w", id, err)
	}
	kind := "albums"
	if opt.onlyFavorites {
		kind = "favorites"
	}
	log.Printf("User %s: %s, %d %s", user.ID, user.Name, len(albumIDs), kind)
	output := opt.output
	if opt.groupByUser {
		output = filepath.Join(output, safeSegment(user.Name))
	}
	userApp := app{client: client, downloader: a.downloader.WithWorkers(workers), workers: workers}
	_, err = userApp.downloadAlbums(ctx, albumIDs, opt, output)
	return err
}

func pause(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (a app) search(ctx context.Context, opt options) error {
	albums, err := a.client.SearchAlbums(ctx, strings.TrimSpace(opt.keyword), opt.sorting, opt.page, opt.maxPage)
	if err != nil {
		return err
	}
	printAlbums(albums)
	if !opt.searchDownload {
		return nil
	}
	ids := make([]string, 0, len(albums))
	for _, album := range albums {
		ids = append(ids, album.ID)
	}
	_, err = a.downloadAlbums(ctx, ids, opt, opt.output)
	return err
}

func printAlbums(albums []luscious.Album) {
	fmt.Printf("%-12s %-40s %-24s %8s %8s\n", "ID", "Title", "Author", "Pictures", "GIFs")
	for _, album := range albums {
		fmt.Printf("%-12s %-40s %-24s %8d %8d\n", album.ID, clip(album.Title, 40), clip(album.Author, 24), album.Pictures, album.AnimatedPictures)
	}
}

func clip(value string, max int) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\t", " ")
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max-1]) + "…"
}

func albumFolderName(album luscious.Album, format string) string {
	title := invalidTitleChars.ReplaceAllString(album.Title, "_")
	if title == "" {
		title = "album-" + album.ID
	}
	name := strings.NewReplacer(
		"%i", album.ID,
		"%t", title,
		"%a", invalidTitleChars.ReplaceAllString(album.Author, "_"),
		"%p", strconv.Itoa(album.Pictures),
		"%g", strconv.Itoa(album.AnimatedPictures),
	).Replace(format)
	name = strings.ReplaceAll(name, "[]", "")
	return safeSegment(name)
}

func safeSegment(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = invalidFolderChars.ReplaceAllString(value, "_")
	value = strings.TrimSpace(strings.Trim(value, "."))
	if value == "" || value == ".." {
		return "download"
	}
	return value
}

func defaultWorkers() int {
	workers := runtime.NumCPU() * 4
	if workers < 16 {
		return 16
	}
	if workers > 64 {
		return 64
	}
	return workers
}

func newDefaultSettings() settings {
	return settings{Directory: "./albums/", Pool: defaultWorkers(), Retries: 5, Timeout: 30, FoldernameFormat: "[%i]%t"}
}

func optionsFromSettings(s settings) options {
	output := s.Directory
	if output == "" {
		output = "./albums/"
	}
	output, _ = filepath.Abs(output)
	threads := s.Pool
	if threads < 1 {
		threads = defaultWorkers()
	} else if threads > maxWorkers {
		threads = maxWorkers
	}
	timeout := s.Timeout
	if timeout < 1 {
		timeout = 30
	}
	format := s.FoldernameFormat
	if format == "" {
		format = "[%i]%t"
	}
	return options{output: output, threads: threads, retries: s.Retries, timeout: timeout, delay: s.Delay, nameFormat: format,
		makePDF: s.GenPDF, makeCBZ: s.GenCBZ, removeOrigin: s.RemoveOrigin, groupByUser: s.GroupByUser,
		page: 1, maxPage: 1, sorting: "date_trending"}
}

func downloadOptions(opt options) app {
	workers := opt.threads
	if workers < 1 {
		workers = defaultWorkers()
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}
	network := netx.New(time.Duration(opt.timeout)*time.Second, workers, opt.sessionCookie)
	return app{
		client:     luscious.NewClient(network.HTTP, network.Limiter, workers),
		downloader: downloader.New(network.HTTP, network.Limiter, workers, opt.retries),
		workers:    workers,
	}
}

func readInput(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func runMenu(ctx context.Context, sessionCookie string) error {
	if err := createMenuFiles(); err != nil {
		return err
	}
	logFile, err := os.OpenFile("logs.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	previousLogOutput := log.Writer()
	log.SetOutput(menuLogWriter{console: previousLogOutput, file: logFile})
	defer func() {
		log.SetOutput(previousLogOutput)
		logFile.Close()
	}()
	settings := loadSettings()
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Luscious Downloader\n1 - Download albums by URL or ID.\n2 - Download all user albums.\n3 - Download user favorites.\n4 - Search albums.\n5 - Download albums from list.txt.\n6 - Settings.\n0 - Exit.\n")
		choice, err := readInput(reader, "> ")
		if err != nil {
			return err
		}
		opt := optionsFromSettings(settings)
		opt.sessionCookie = sessionCookie
		switch choice {
		case "1":
			input, err := readInput(reader, "Album URL or ID (0 to go back): ")
			if err != nil {
				return err
			}
			if input == "0" {
				continue
			}
			ids, parseErr := parseIDs(input, "album")
			completed, runErr := downloadOptions(opt).downloadAlbums(ctx, ids, opt, opt.output)
			for _, id := range completed {
				appendCompleted("album", id)
			}
			showMenuError(errors.Join(parseErr, runErr))
		case "2", "3":
			input, err := readInput(reader, "User URL or ID (0 to go back): ")
			if err != nil {
				return err
			}
			if input == "0" {
				continue
			}
			ids, parseErr := parseIDs(input, "user")
			opt.onlyFavorites = choice == "3"
			completed, runErr := downloadOptions(opt).downloadUsers(ctx, ids, opt)
			for _, id := range completed {
				appendCompleted("user", id)
			}
			showMenuError(errors.Join(parseErr, runErr))
		case "4":
			keyword, err := readInput(reader, "Search keyword (0 to go back): ")
			if err != nil {
				return err
			}
			if keyword == "0" || keyword == "" {
				continue
			}
			pageText, err := readInput(reader, "Starting page (blank for 1): ")
			if err != nil {
				return err
			}
			maxText, err := readInput(reader, "Last page (blank for 1): ")
			if err != nil {
				return err
			}
			opt.page = parsePositiveOr(pageText, 1)
			opt.maxPage = parsePositiveOr(maxText, 1)
			if opt.maxPage < opt.page {
				opt.maxPage = opt.page
			}
			opt.keyword = keyword
			downloading, err := readInput(reader, "Download results? (y/N): ")
			if err != nil {
				return err
			}
			opt.searchDownload = strings.EqualFold(downloading, "y")
			showMenuError(downloadOptions(opt).search(ctx, opt))
		case "5":
			showMenuError(downloadOptions(opt).downloadList(ctx, opt, true))
		case "6":
			settings, err = editSettings(reader, settings)
			if err != nil {
				return err
			}
		case "0":
			return nil
		default:
			fmt.Println("Invalid option.")
		}
	}
}

type menuLogWriter struct {
	console io.Writer
	file    io.Writer
}

func (w menuLogWriter) Write(data []byte) (int, error) {
	n, err := w.console.Write(data)
	if strings.Contains(string(data), "ERROR") {
		if _, fileErr := w.file.Write(data); err == nil {
			err = fileErr
		}
	}
	return n, err
}

func parsePositiveOr(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func parseNonNegativeOr(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func showMenuError(err error) {
	if err != nil {
		log.Printf("ERROR: %v", err)
	}
}

func createMenuFiles() error {
	for _, name := range []string{"list.txt", "list_completed.txt"} {
		file, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if _, err := os.Stat("config.json"); errors.Is(err, os.ErrNotExist) {
		return saveSettings(newDefaultSettings())
	}
	return nil
}

func loadSettings() settings {
	current := newDefaultSettings()
	data, err := os.ReadFile("config.json")
	if err != nil {
		return current
	}
	if err := json.Unmarshal(data, &current); err != nil {
		log.Printf("WARNING: config.json: %v", err)
	}
	return current
}

func saveSettings(current settings) error {
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("config.json", append(data, '\n'), 0o644)
}

func editSettings(reader *bufio.Reader, current settings) (settings, error) {
	for {
		fmt.Printf("1 - Directory [%s]\n2 - Download workers [%d]\n3 - Retries [%d]\n4 - Timeout seconds [%d]\n5 - Delay seconds [%d]\n6 - Folder format [%s]\n7 - Generate PDF [%t]\n8 - Generate CBZ [%t]\n9 - Remove source folder [%t]\n10 - Group by user [%t]\n0 - Back\n",
			current.Directory, current.Pool, current.Retries, current.Timeout, current.Delay, current.FoldernameFormat,
			current.GenPDF, current.GenCBZ, current.RemoveOrigin, current.GroupByUser)
		choice, err := readInput(reader, "> ")
		if err != nil {
			return current, err
		}
		if choice == "0" {
			return current, saveSettings(current)
		}
		if choice == "7" {
			current.GenPDF = !current.GenPDF
		} else if choice == "8" {
			current.GenCBZ = !current.GenCBZ
		} else if choice == "9" {
			current.RemoveOrigin = !current.RemoveOrigin
		} else if choice == "10" {
			current.GroupByUser = !current.GroupByUser
		} else {
			value, err := readInput(reader, "New value: ")
			if err != nil {
				return current, err
			}
			switch choice {
			case "1":
				if value == "1" {
					current.Directory = "./albums/"
				} else if value != "" && value != "0" {
					current.Directory = value
				}
			case "2":
				current.Pool = parsePositiveOr(value, current.Pool)
			case "3":
				current.Retries = parseNonNegativeOr(value, current.Retries)
			case "4":
				current.Timeout = parsePositiveOr(value, current.Timeout)
			case "5":
				if n, err := strconv.Atoi(value); err == nil && n >= 0 {
					current.Delay = n
				}
			case "6":
				if value != "" {
					current.FoldernameFormat = value
				}
			default:
				fmt.Println("Invalid option.")
			}
		}
		if err := saveSettings(current); err != nil {
			return current, err
		}
	}
}

func (a app) downloadList(ctx context.Context, opt options, trackProgress bool) error {
	items, err := readList("list.txt")
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("list.txt is empty")
	}
	var ids []string
	byID := make(map[string][]string)
	var parseErrors []error
	for _, item := range items {
		id, err := extractID(item, "album")
		if err != nil {
			parseErrors = append(parseErrors, err)
			continue
		}
		ids = append(ids, id)
		byID[id] = append(byID[id], item)
	}
	completed, runErr := a.downloadAlbums(ctx, ids, opt, opt.output)
	if trackProgress {
		for _, id := range completed {
			appendCompleted("album", id)
		}
		if len(completed) > 0 {
			removeListItems("list.txt", byID, completed)
		}
	}
	return errors.Join(errors.Join(parseErrors...), runErr)
}

func readList(name string) ([]string, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var items []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		for _, value := range strings.Split(scanner.Text(), ",") {
			item := strings.TrimSpace(value)
			if item != "" && !seen[item] {
				items = append(items, item)
				seen[item] = true
			}
		}
	}
	return items, scanner.Err()
}

func appendCompleted(kind, id string) {
	file, err := os.OpenFile("list_completed.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("ERROR: list_completed.txt: %v", err)
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s-%s\n", kind, id)
}

func removeListItems(name string, byID map[string][]string, ids []string) {
	remove := make(map[string]bool)
	for _, id := range ids {
		for _, input := range byID[id] {
			remove[input] = true
		}
	}
	items, err := readList(name)
	if err != nil {
		log.Printf("ERROR: read %s: %v", name, err)
		return
	}
	var remaining []string
	for _, item := range items {
		if !remove[item] {
			remaining = append(remaining, item)
		}
	}
	if err := os.WriteFile(name, []byte(strings.Join(remaining, "\n")), 0o644); err != nil {
		log.Printf("ERROR: update %s: %v", name, err)
	}
}
