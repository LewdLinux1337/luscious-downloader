package luscious

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/LewdLinux1337/luscious-downloader/internal/netx"
)

const endpoint = "https://members.luscious.net/graphql/nobatch/"

type Client struct {
	http    *http.Client
	limiter *netx.Limiter
	workers int
}

type Album struct {
	ID               string
	Title            string
	Author           string
	Pictures         int
	AnimatedPictures int
}

type User struct {
	ID        string
	Name      string
	Albums    int
	Favorites int
}

type Picture struct {
	ID  string
	URL string
}

type graphQLRequest struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Errors []graphQLError  `json:"errors"`
	Data   json.RawMessage `json:"data"`
}

type graphQLID string

type pageInfo struct {
	Page        int  `json:"page"`
	HasNextPage bool `json:"has_next_page"`
	TotalPages  int  `json:"total_pages"`
}

const mediaBaseURL = "https://www.luscious.net/"

func parallelPages[T any](ctx context.Context, workers, first, last int, fetch func(context.Context, int) (T, error)) ([]T, error) {
	if last < first {
		return nil, nil
	}
	count := last - first + 1
	if workers > count {
		workers = count
	}
	if workers < 1 {
		workers = 1
	}
	results := make([]T, count)
	errs := make([]error, count)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				index := page - first
				if err := ctx.Err(); err != nil {
					errs[index] = err
					continue
				}
				results[index], errs[index] = fetch(ctx, page)
			}
		}()
	}
	for page := first; page <= last; page++ {
		select {
		case jobs <- page:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return results, errors.Join(errs...)
}

func (id *graphQLID) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*id = graphQLID(value)
		return nil
	}
	*id = graphQLID(strings.TrimSpace(string(data)))
	return nil
}

func NewClient(client *http.Client, limiter *netx.Limiter, workers int) *Client {
	if workers < 1 {
		workers = 1
	}
	return &Client{http: client, limiter: limiter, workers: workers}
}

func (c *Client) WithWorkers(workers int) *Client {
	if workers < 1 {
		workers = 1
	}
	return &Client{http: c.http, limiter: c.limiter, workers: workers}
}

func (c *Client) request(ctx context.Context, operation, query string, variables map[string]any, target any) error {
	body, err := json.Marshal(graphQLRequest{OperationName: operation, Query: query, Variables: variables})
	if err != nil {
		return err
	}
	requestURL := endpoint + "?operationName=" + url.QueryEscape(operation)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "luscious-downloader-go/1.0")
	if err := c.limiter.Acquire(ctx); err != nil {
		return err
	}
	response, err := c.http.Do(req)
	if err != nil {
		c.limiter.Release()
		return fmt.Errorf("%s request: %w", operation, err)
	}
	defer func() {
		response.Body.Close()
		c.limiter.Release()
	}()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", operation, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned HTTP %s: %s", operation, response.Status, strings.TrimSpace(string(data)))
	}
	var envelope graphQLResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", operation, err)
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, item := range envelope.Errors {
			messages = append(messages, item.Message)
		}
		return fmt.Errorf("%s GraphQL error: %s", operation, strings.Join(messages, "; "))
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("%s returned no data", operation)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("decode %s data: %w", operation, err)
	}
	return nil
}

func (c *Client) Album(ctx context.Context, id string) (Album, error) {
	const query = `query AlbumGet($id: ID!) {
		album { get(id: $id) {
			... on Album { id title number_of_pictures number_of_animated_pictures created_by { display_name name } }
			... on MutationError { errors { code message } }
		} }
	}`
	var data struct {
		Album struct {
			Get struct {
				ID               graphQLID `json:"id"`
				Title            string    `json:"title"`
				Pictures         int       `json:"number_of_pictures"`
				AnimatedPictures int       `json:"number_of_animated_pictures"`
				CreatedBy        struct {
					DisplayName string `json:"display_name"`
					Name        string `json:"name"`
				} `json:"created_by"`
				Errors []graphQLError `json:"errors"`
			} `json:"get"`
		} `json:"album"`
	}
	if err := c.request(ctx, "AlbumGet", query, map[string]any{"id": id}, &data); err != nil {
		return Album{}, err
	}
	result := data.Album.Get
	if len(result.Errors) > 0 {
		return Album{}, fmt.Errorf("album %s: %s", id, result.Errors[0].Message)
	}
	if result.ID == "" {
		return Album{}, fmt.Errorf("album %s not found", id)
	}
	author := result.CreatedBy.DisplayName
	if author == "" {
		author = result.CreatedBy.Name
	}
	return Album{ID: string(result.ID), Title: result.Title, Author: author, Pictures: result.Pictures, AnimatedPictures: result.AnimatedPictures}, nil
}

func (c *Client) AlbumPictures(ctx context.Context, id string) ([]Picture, error) {
	const query = `query AlbumListOwnPictures($input: PictureListInput!) {
		picture { list(input: $input) {
			info { page has_next_page total_pages }
			items { id url_to_original url_to_video url }
		} }
	}`
	type item struct {
		ID       graphQLID `json:"id"`
		Original string    `json:"url_to_original"`
		Video    string    `json:"url_to_video"`
		URL      string    `json:"url"`
	}
	type picturePage struct {
		Info  pageInfo `json:"info"`
		Items []item   `json:"items"`
	}
	fetchPage := func(ctx context.Context, page int) (picturePage, error) {
		var data struct {
			Picture struct {
				List picturePage `json:"list"`
			} `json:"picture"`
		}
		variables := map[string]any{"input": map[string]any{
			"filters": []map[string]string{{"name": "album_id", "value": id}},
			"display": "date_newest",
			"page":    page,
		}}
		if err := c.request(ctx, "AlbumListOwnPictures", query, variables, &data); err != nil {
			return picturePage{}, err
		}
		return data.Picture.List, nil
	}
	first, err := fetchPage(ctx, 1)
	if err != nil {
		return nil, err
	}
	pages := []picturePage{first}
	if first.Info.HasNextPage {
		if first.Info.TotalPages > 1 {
			rest, err := parallelPages(ctx, c.workers, 2, first.Info.TotalPages, fetchPage)
			if err != nil {
				return nil, err
			}
			pages = append(pages, rest...)
		} else {
			for page := 2; ; page++ {
				next, err := fetchPage(ctx, page)
				if err != nil {
					return nil, err
				}
				pages = append(pages, next)
				if !next.Info.HasNextPage {
					break
				}
			}
		}
	}
	var pictures []Picture
	for _, page := range pages {
		for _, picture := range page.Items {
			downloadURL := picture.Original
			if downloadURL == "" {
				downloadURL = picture.Video
			}
			if downloadURL == "" {
				downloadURL = picture.URL
			}
			if downloadURL != "" {
				mediaURL, err := resolveMediaURL(downloadURL)
				if err != nil {
					return nil, fmt.Errorf("picture %s media URL: %w", picture.ID, err)
				}
				pictures = append(pictures, Picture{ID: string(picture.ID), URL: mediaURL})
			}
		}
	}
	return pictures, nil
}

func resolveMediaURL(value string) (string, error) {
	base, _ := url.Parse(mediaBaseURL)
	reference, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(reference).String(), nil
}

func (c *Client) SearchAlbums(ctx context.Context, keyword, sorting string, page, maxPage int) ([]Album, error) {
	const query = `query AlbumList($input: AlbumListInput!) {
		album { list(input: $input) {
			info { page has_next_page total_pages }
			items { id title number_of_pictures number_of_animated_pictures created_by { display_name name } }
		} }
	}`
	type searchItem struct {
		ID               graphQLID `json:"id"`
		Title            string    `json:"title"`
		Pictures         int       `json:"number_of_pictures"`
		AnimatedPictures int       `json:"number_of_animated_pictures"`
		CreatedBy        struct {
			DisplayName string `json:"display_name"`
			Name        string `json:"name"`
		} `json:"created_by"`
	}
	type searchPage struct {
		Info  pageInfo     `json:"info"`
		Items []searchItem `json:"items"`
	}
	fetchPage := func(ctx context.Context, current int) (searchPage, error) {
		var data struct {
			Album struct {
				List searchPage `json:"list"`
			} `json:"album"`
		}
		variables := map[string]any{"input": map[string]any{
			"display": sorting,
			"filters": []map[string]string{
				{"name": "album_type", "value": "pictures"},
				{"name": "search_query", "value": keyword},
			},
			"page": current,
		}}
		if err := c.request(ctx, "AlbumList", query, variables, &data); err != nil {
			return searchPage{}, err
		}
		return data.Album.List, nil
	}
	first, err := fetchPage(ctx, page)
	if err != nil {
		return nil, err
	}
	pages := []searchPage{first}
	if first.Info.HasNextPage && page < maxPage {
		if first.Info.TotalPages > 0 {
			last := maxPage
			if last > first.Info.TotalPages {
				last = first.Info.TotalPages
			}
			rest, err := parallelPages(ctx, c.workers, page+1, last, fetchPage)
			if err != nil {
				return nil, err
			}
			pages = append(pages, rest...)
		} else {
			for current := page + 1; current <= maxPage; current++ {
				next, err := fetchPage(ctx, current)
				if err != nil {
					return nil, err
				}
				pages = append(pages, next)
				if !next.Info.HasNextPage {
					break
				}
			}
		}
	}
	var albums []Album
	for _, resultPage := range pages {
		for _, item := range resultPage.Items {
			author := item.CreatedBy.DisplayName
			if author == "" {
				author = item.CreatedBy.Name
			}
			albums = append(albums, Album{ID: string(item.ID), Title: item.Title, Author: author, Pictures: item.Pictures, AnimatedPictures: item.AnimatedPictures})
		}
	}
	return albums, nil
}

func (c *Client) User(ctx context.Context, id string) (User, error) {
	const query = `query ProfileGet($user_id: ID!) {
		userprofile { get(user_id: $user_id) {
			... on UserProfile { id user { id name } number_of_albums number_of_favorite_albums }
			... on MutationError { errors { code message } }
		} }
	}`
	var data struct {
		UserProfile struct {
			Get struct {
				ID   graphQLID `json:"id"`
				User struct {
					Name string `json:"name"`
				} `json:"user"`
				Albums    int            `json:"number_of_albums"`
				Favorites int            `json:"number_of_favorite_albums"`
				Errors    []graphQLError `json:"errors"`
			} `json:"get"`
		} `json:"userprofile"`
	}
	if err := c.request(ctx, "ProfileGet", query, map[string]any{"user_id": id}, &data); err != nil {
		return User{}, err
	}
	result := data.UserProfile.Get
	if len(result.Errors) > 0 {
		return User{}, fmt.Errorf("user %s: %s", id, result.Errors[0].Message)
	}
	if result.ID == "" {
		return User{}, fmt.Errorf("user %s not found", id)
	}
	return User{ID: string(result.ID), Name: result.User.Name, Albums: result.Albums, Favorites: result.Favorites}, nil
}

func (c *Client) UserAlbums(ctx context.Context, id string, favorites bool) ([]string, error) {
	query := `query AlbumList($input: AlbumListInput!) {
		album { list(input: $input) { info { page has_next_page total_pages } items { id } } }
	}`
	filterName := "created_by_id"
	if favorites {
		filterName = "favorite_by_user_id"
	}
	type albumPage struct {
		Info  pageInfo `json:"info"`
		Items []struct {
			ID graphQLID `json:"id"`
		} `json:"items"`
	}
	fetchPage := func(ctx context.Context, page int) (albumPage, error) {
		var data struct {
			Album struct {
				List albumPage `json:"list"`
			} `json:"album"`
		}
		variables := map[string]any{"input": map[string]any{
			"display": "date_newest",
			"filters": []map[string]string{{"name": filterName, "value": id}},
			"page":    page,
		}}
		if err := c.request(ctx, "AlbumList", query, variables, &data); err != nil {
			return albumPage{}, err
		}
		return data.Album.List, nil
	}
	first, err := fetchPage(ctx, 1)
	if err != nil {
		return nil, err
	}
	pages := []albumPage{first}
	if first.Info.HasNextPage {
		if first.Info.TotalPages > 1 {
			rest, err := parallelPages(ctx, c.workers, 2, first.Info.TotalPages, fetchPage)
			if err != nil {
				return nil, err
			}
			pages = append(pages, rest...)
		} else {
			for page := 2; ; page++ {
				next, err := fetchPage(ctx, page)
				if err != nil {
					return nil, err
				}
				pages = append(pages, next)
				if !next.Info.HasNextPage {
					break
				}
			}
		}
	}
	var albums []string
	for _, resultPage := range pages {
		for _, item := range resultPage.Items {
			albums = append(albums, string(item.ID))
		}
	}
	return albums, nil
}
