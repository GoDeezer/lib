package deezer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

type ErrUnexpectedStatusCode struct{ Code int }

func (err ErrUnexpectedStatusCode) Error() string {
	return fmt.Sprintf("Deezer returned non-2XX code %d", err.Code)
}

const apiURL = "https://www.deezer.com/ajax/gw-light.php"

type apiMethod string

const (
	getUserData         apiMethod = "deezer.getUserData"
	pageSearch          apiMethod = "deezer.pageSearch"
	songGetData         apiMethod = "song.getData"
	songGetLyrics       apiMethod = "song.getLyrics"
	songListByAlbum     apiMethod = "song.getListByAlbum"
	albumGetData        apiMethod = "album.getData"
	artistGetData       apiMethod = "artist.getData"
	albumGetDiscography apiMethod = "album.getDiscography"
)

type songListByAlbumBody struct {
	ID    string `json:"alb_id"`
	Limit int    `json:"nb"`
}

type albumGetDiscographyBody struct {
	ArtistID   string `json:"art_id"`
	Language   string `json:"lang"`
	FilterRole []int  `json:"filter_role_id"`
	Limit      int    `json:"nb"`
	LimitSongs int    `json:"nb_songs"`
	Start      int    `json:"start"`
}

type userData struct {
	CheckForm string `json:"checkForm"`
}

type response struct {
	Results json.RawMessage `json:"results"`
}

type multiSongResponse struct {
	Data []Song `json:"data"`
}

type multiAlbumResponse struct {
	Data []Album `json:"data"`
}

type pageSearchBody struct {
	Query  string `json:"query"`
	Filter string `json:"filter"`
	Output string `json:"output"`
	Start  int    `json:"start"`
	Limit  int    `json:"nb"`
}

type SearchResponse struct {
	Artists struct {
		Data []Artist `json:"data"`
	} `json:"ARTIST"`
	Songs struct {
		Data []Song `json:"data"`
	} `json:"TRACK"`
	Albums struct {
		Data []Album `json:"data"`
	} `json:"ALBUM"`
}

type Client struct {
	*http.Client
	Arl string
}

// NewClient returns a Deezer client with
// the given arl (used for authentication)
// this arl can be gotten by following these instructions:
// https://notabug.org/RemixDevs/DeezloaderRemix/wiki/Login+via+userToken
func NewClient(arl string) *Client {
	return NewClientWithHTTPClient(arl, new(http.Client))
}

// NewClientWithHTTPClient returns a Deezer client with the given arl.
// This is useful for setting a timeout for requests made by the Client.
// It will modify the given http.Client's Jar.
func NewClientWithHTTPClient(arl string, client *http.Client) *Client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	url, err := url.Parse(apiURL)
	if err != nil {
		panic(err)
	}
	jar.SetCookies(url, []*http.Cookie{
		{
			Name:  "arl",
			Value: arl,
		},
	})
	client.Jar = jar
	return &Client{client, arl}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/81.0.4044.129 Safari/537.36")
	req.Header.Add("Cache-Control", "max-age=0")
	req.Header.Add("Accept-Language", "en-US,en;q=0.9,en-US;q=0.8,en;q=0.7")
	return c.Client.Do(req)
}

func (c *Client) apiDo(method apiMethod, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return nil, err
	}
	var token string
	if method != getUserData {
		t, err := c.csrfToken()
		if err != nil {
			return nil, err
		}
		token = t
	} else {
		token = "null"
	}
	qs := url.Values{}
	qs.Add("api_version", "1.0")
	qs.Add("api_token", token)
	qs.Add("input", "3")
	qs.Add("method", string(method))
	req.URL.RawQuery = qs.Encode()
	req.AddCookie(&http.Cookie{Name: "arl", Value: c.Arl})
	r, e := c.do(req)
	if r.StatusCode < 200 || r.StatusCode > 299 {
		return nil, ErrUnexpectedStatusCode{r.StatusCode}
	}
	return r, e
}

func (c *Client) apiDoJSON(method apiMethod, body interface{}, v interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r := bytes.NewBuffer(b)
	resp, err := c.apiDo(method, r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	var jresp response
	err = dec.Decode(&jresp)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jresp.Results, &v)
	if err != nil &&
		// Hacky, but required to be able to unmarshal
		// the awful JSON that PHP produces
		!strings.Contains(err.Error(),
			"json: cannot unmarshal array into Go struct field") {
		return err
	}
	return nil
}

func (c *Client) get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) csrfToken() (string, error) {
	var udata userData
	err := c.apiDoJSON(getUserData, nil, &udata)
	return udata.CheckForm, err
}

// Song fetches a Song.
func (c *Client) Song(id string) (*Song, error) {
	var song Song
	body := struct {
		ID string `json:"sng_id"`
	}{id}
	err := c.apiDoJSON(songGetData, body, &song)
	if err != nil {
		return nil, err
	}
	return &song, nil
}

// Lyrics fetches a song's lyrics. The ID provided can be from
// a Song's LyricsID field.
func (c *Client) Lyrics(id string) (*Lyrics, error) {
	var song Lyrics
	body := struct {
		ID string `json:"sng_id"`
	}{id}
	err := c.apiDoJSON(songGetLyrics, body, &song)
	if err != nil {
		return nil, err
	}
	return &song, nil
}

// Album fetches an Album.
func (c *Client) Album(id string) (*Album, error) {
	var album Album
	body := struct {
		ID string `json:"alb_id"`
	}{id}
	err := c.apiDoJSON(albumGetData, body, &album)
	if err != nil {
		return nil, err
	}
	return &album, nil
}

// Artist fetches an Artist.
func (c *Client) Artist(id string) (*Artist, error) {
	var artist Artist
	body := struct {
		ID string `json:"art_id"`
	}{id}
	err := c.apiDoJSON(artistGetData, body, &artist)
	if err != nil {
		return nil, err
	}
	return &artist, nil
}

// SongsByAlbum fetches up to songLimit songs on an album.
// If you want to fetch all of the songs on an album,
// use a songLimit of -1.
func (c *Client) SongsByAlbum(id string, songLimit int) ([]Song, error) {
	var songs multiSongResponse
	body := songListByAlbumBody{id, songLimit}
	err := c.apiDoJSON(songListByAlbum, body, &songs)
	return songs.Data, err
}

// AlbumsBy fetches albums in an artist's discography.
func (c *Client) AlbumsByArtist(id string) ([]Album, error) {
	var albums multiAlbumResponse
	body := albumGetDiscographyBody{id, "us", []int{0}, 500, 300, 0}
	err := c.apiDoJSON(albumGetDiscography, body, &albums)
	return albums.Data, err
}

// Search searches for artists/albums/songs.
func (c *Client) Search(query, filter, output string, start, limit int) (*SearchResponse, error) {
	var resp SearchResponse
	body := pageSearchBody{query, filter, output, start, limit}
	err := c.apiDoJSON(pageSearch, body, &resp)
	return &resp, err
}

// AvailableQualities returns the available qualities for download
// of a song.
func (c *Client) AvailableQualities(song Song) []Quality {
	var qualities []Quality
	if c.IsQualityAvailable(song, MP3128) {
		qualities = append(qualities, MP3128)
	}
	if c.IsQualityAvailable(song, MP3320) {
		qualities = append(qualities, MP3320)
	}
	if c.IsQualityAvailable(song, FLAC) {
		qualities = append(qualities, FLAC)
	}
	return qualities
}

// IsQualityAvailable returns whether or not a song is available
// to download for a song.
func (c *Client) IsQualityAvailable(song Song, quality Quality) bool {
	url := song.DownloadURL(quality)
	if url == "" {
		return false
	}
	resp, err := c.get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false
	}
	return true
}

type songDownloadReader struct {
	r    *DecryptingReader
	body io.ReadCloser
}

func (s songDownloadReader) Read(p []byte) (int, error) {
	return s.r.Read(p)
}

func (s songDownloadReader) Close() error {
	return s.body.Close()
}

// Write writes a song from Deezer at a given quality to w.
func (s Song) Write(w io.Writer, quality Quality) error {
	url := s.DownloadURL(quality)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ErrUnexpectedStatusCode{resp.StatusCode}
	}
	r, err := NewDecryptingReader(resp.Body, s.ID)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

// Download returns an io.ReadCloser from which the song can be read at the
// given quality. This function is deprecated and it is preferred to use song.Write
func (c *Client) Download(song Song, quality Quality) (io.ReadCloser, error) {
	url := song.DownloadURL(quality)
	resp, err := c.get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, ErrUnexpectedStatusCode{resp.StatusCode}
	}
	r, err := NewDecryptingReader(resp.Body, song.ID)
	return songDownloadReader{r, resp.Body}, err
}
