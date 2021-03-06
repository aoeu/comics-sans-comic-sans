package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// An RSS represents an RSS feed read from an XML file.
type RSS struct {
	Version string `xml:"version,attr"`
	// Using shorthand of just "Channel" doesn't work here, e.g.
	// Channel `xml:"channel"`
	Channel Channel `xml:"channel"`
}

// A Channel represents the contents of an RSS channel ("feed").
type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	LastBuildDate string `xml:"lastBuildDate"`
	Items         []Item `xml:"item"`
}

// An Item represents a single item from an RSS channel.
type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Guid        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// A Comic represents a single webcomic image and related metadata.
type Comic struct {
	Title        string
	Link         string
	ImageURL     string
	ImageComment string
	Date         string
	UnixDate     int64
	PubMsg       string
}

// sanitize sets fields of a Comic to preferred defaults.
func (c *Comic) sanitize() {
	if (*c).Title == "." { // Hack: yahoo pipes will not allow blank title.
		(*c).Title = ""
	}
	// Do not allow repition of title in image comment.
	if (*c).ImageComment == (*c).Title {
		(*c).ImageComment = ""
	}
}

// A ComicSeries represents multiple webcomics published by a single site
// or author.
type ComicSeries struct {
	SeriesTitle string
	SiteURL     string
	Description string
	Index       int // For the front-end to keep track of Comic to display.
	Comics      []Comic
}

// sanitize sets fields of a ComicSeries to preferred defaults.
func (c *ComicSeries) sanitize() {
	// Hacks to accomodate bogus data values from yahoo pipes.
	if (*c).Description == "." || (*c).Description == "Pipes Output" {
		(*c).Description = " "
	}
	// Do not allow repetition of the comic series title.
	for _, comic := range (*c).Comics {
		if comic.Title == (*c).SeriesTitle {
			comic.Title = ""
		}
	}
}

// A ComicMetadata represents metadata used for fetching and parsing a
// ComicSeries and its Comics.
type ComicMetadata struct {
	URL        string
	ImgAttrs   map[string]string
	ImgComment string
	Name       string
	Category   string
	RSSFeed    *RSS
}

// downloadFeed requests an RSS feed with a URL and stores response as a field.
func (c *ComicMetadata) downloadFeed(wg *sync.WaitGroup, errs chan error) {
	defer wg.Done()
	resp, err := download(c.URL, "5s") // TODO(aoeu): Make timeout configurable?
	if err != nil {
		errs <- fmt.Errorf("did not receive HTTP GET response: %v", err)
		return
	}
	if resp.StatusCode != 200 {
		errs <- fmt.Errorf("bad HTTP Response %d for %s\n", resp.StatusCode, c.URL)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errs <- fmt.Errorf("could not read response body: %v\n", err)
		return
	}
	rss := new(RSS)
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = func(contentType string, in io.Reader) (out io.Reader, err error) {
		// Over the years, the arugment order has flipped.
		out, err = charset.NewReader(in, contentType)
		return out, err
	}
	err = decoder.Decode(rss)
	if err != nil {
		errs <- fmt.Errorf("problem unmarshalling XML in response of '%v': %v", c.URL, err)
		return
	}
	c.RSSFeed = rss
}

func download(url, timeoutLen string) (*http.Response, error) {
	timeout, err := time.ParseDuration(timeoutLen)
	if err != nil {
		s := "could not download URL '%v' due to invalide timeout len: %v"
		return nil, fmt.Errorf(s, url, timeoutLen)
	}
	responses := make(chan *http.Response)
	errors := make(chan error)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			errors <- err
			return
		}
		responses <- resp
	}()
	select {
	case r := <-responses:
		return r, nil
	case err := <-errors:
		return nil, err
	case <-time.After(timeout):
		s := "the URL '%v' did not download after %v"
		return nil, fmt.Errorf(s, url, timeout)
	}
}

// parseDate calculates, formats, and returns time information about an
// RSS "pubDate" node.
func parseDate(pubDate string) (
	newPubDate string, unixDate int64, pubMsg string, err error) {
	dateTime, err := time.Parse(time.RFC1123Z, pubDate)
	if err != nil {
		dateTime, err = time.Parse(time.RFC1123, pubDate)
	}
	dateTime = dateTime.Local()
	newPubDate = dateTime.Format(time.RFC1123)
	unixDate = dateTime.Unix()
	pubMsg = lastUpdate(unixDate, time.Now().Unix())
	return
}

// lastUpdate creates an English sentence stating how much time has elapsed
// between 2 points in Unix time.
func lastUpdate(then, now int64) (lastUpdate string) {
	var hourLength int64 = 60 * 60
	var dayLength int64 = hourLength * 24
	diffTime := now - then
	lastUpdate = "Published "
	if diffTime > dayLength {
		days := diffTime / dayLength
		lastUpdate += fmt.Sprintf("%d day", days)
		if days > 1 {
			lastUpdate += "s"
		}
		lastUpdate += " ago on"
	} else if diffTime > hourLength {
		lastUpdate += fmt.Sprintf("%d hours ago on", diffTime/hourLength)
	} else {
		lastUpdate += "less than 1 hour ago on"
	}
	return
}

// parseComic obtains Comic data from a single RSS <item> node.
func parseComic(item Item, commentAttrName string) (c Comic, err error) {
	c = Comic{Title: item.Title, Link: item.Link}
	// TODO: Why ignore error?
	c.Date, c.UnixDate, c.PubMsg, _ = parseDate(item.PubDate)
	//item.Description = html.EscapeString(item.Description)
	node, err := html.Parse(strings.NewReader(item.Description))
	if err != nil {
		return Comic{}, err
	}
	doc := goquery.NewDocumentFromNode(node)
	if err != nil {
		return Comic{}, err
	}
	doc.Find("img").Each(func(i int, img *goquery.Selection) {
		if c.ImageURL == "" {
			c.ImageURL, _ = img.Attr("src")
			c.ImageComment, _ = img.Attr(commentAttrName)
		} else {
			return // Keep going until we set any image.
		}
	})
	c.sanitize()
	return
}

// parseComicSeries parses an entire series of Comics from an RSS feed.
func parseComicSeries(feed *RSS, commentAttrName string) (ComicSeries, error) {
	if commentAttrName == "" {
		commentAttrName = "alt"
	}
	series := &ComicSeries{
		SeriesTitle: feed.Channel.Title,
		SiteURL:     feed.Channel.Link,
		Description: feed.Channel.Description,
		Index:       0,
	}
	lastBuildDate := feed.Channel.LastBuildDate
	var comics []Comic
	for _, item := range feed.Channel.Items {
		comic, err := parseComic(item, commentAttrName)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if comic.UnixDate < 0 { // Hack: Some comics don't have pubDate on items.
			comic.Date, comic.UnixDate, comic.PubMsg, _ = parseDate(lastBuildDate)
		}
		comics = append(comics, comic)
	}
	series.Comics = comics
	series.sanitize()
	return *series, nil
}

// parseFeeds obtains ComicSeries data for each RSS feed via ComicMetaData.
func parseFeeds(metaData []*ComicMetadata) (comics []ComicSeries) {
	for _, md := range metaData {
		if md.RSSFeed == nil {
			fmt.Fprintf(os.Stderr, "no RSS feed downloaded for %v", md.URL)
			continue // TODO(aoeu): Is there a better approach?
		}
		var comic ComicSeries
		comic, err := parseComicSeries(md.RSSFeed, md.ImgComment)
		if md.Name != "" {
			comic.SeriesTitle = md.Name
		}
		if len(comic.Comics) != 0 {
			comics = append(comics, comic)
		} else {
			fmt.Fprintf(os.Stderr, "no actual comics in feed: %v\n", md.Name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "received error for %s: %v", md.Name, err)
			}
		}
	}
	return
}

// downloadFeeds concurrently downloads and sets RSS feed data for sent
// ComicMetaData.
func downloadFeeds(config []*ComicMetadata) {
	var wg sync.WaitGroup
	errs := make(chan error)
	defer close(errs)
	for _, c := range config {
		wg.Add(1)
		go c.downloadFeed(&wg, errs)
	}
	go func() {
		for err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	wg.Wait()
}

// parseConfig reads a configuration file specifying RSS feeds and
// constructs ComicMetaData structs to represent them.
func parseConfig(filepath string) (out []*ComicMetadata, err error) {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return out, fmt.Errorf("error opening '%v': %v", filepath, err)
	}
	var c []ComicMetadata
	json.Unmarshal(b, &c)
	for i, _ := range c {
		out = append(out, &c[i])
	}
	return out, nil
}

// sortByPubDate sorts a ComicSeries by chronological order of the publication
// date via a basic quicksort algorithm implementation.
func sortByPubDate(in []ComicSeries) (out []ComicSeries) {
	length := len(in)
	if length < 2 {
		return in
	}
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintf(os.Stderr, "quickSort failed: %v\n", err)
		}
	}()
	pivot := (length / 2) - 1
	var lesser, greater []ComicSeries
	for index, _ := range in {
		if index == pivot {
			continue
		}
		time1 := in[index].Comics[0].UnixDate
		time2 := in[pivot].Comics[0].UnixDate
		if time1 <= time2 {
			lesser = append(lesser, in[index])
		} else {
			greater = append(greater, in[index])
		}
	}
	lesserLen := len(lesser)
	out = make([]ComicSeries, lesserLen+1+len(greater))
	copy(out, sortByPubDate(lesser))
	copy(out[lesserLen:], in[pivot:pivot+1])
	copy(out[lesserLen+1:], sortByPubDate(greater))
	return out
}

// reverse reverses the sort order of a ComicSeries.
func reverse(series []ComicSeries) []ComicSeries {
	j := len(series) - 1
	result := make([]ComicSeries, len(series))
	for i, _ := range series {
		result[j-i] = series[i]
	}
	return result
}

// incr increments a number and is only meant for use in the HTML template.
func incr(n int) string { return fmt.Sprintf("%d", n+1) }

// decr increments a number and is only meant for use in the HTML template.
func decr(n int) string { return fmt.Sprintf("%d", n-1) }

// check logs an error and exits if the sent error is not nil.
func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const outputFilename = "index.html"

var port string

func main() {
	flag.StringVar(&port, "port", ":8080", "The port to serve the page on.")
	flag.Parse()

	tmplText, err := ioutil.ReadFile("static/template.html")
	check(err)
	tmplString := string(tmplText)
	funcMap := template.FuncMap{"incr": incr, "decr": decr}
	tmpl, err := template.New("comics").Funcs(funcMap).Parse(tmplString)
	check(err)

	metaData, err := parseConfig("config.json")
	check(err)

	downloadFeeds(metaData)
	comics := parseFeeds(metaData)

	outputFile, err := os.Create(outputFilename)
	check(err)
	comics = reverse(sortByPubDate(comics))
	err = tmpl.Execute(outputFile, comics)
	check(err)

	jsonData, err := json.Marshal(comics)
	check(err)
	err = ioutil.WriteFile("comics.json", jsonData, 0644)
	check(err)

	s := "could not get OS to open '%v': %v\n"
	switch runtime.GOOS {
	case "linux":
		if err := exec.Command("xdg-open", outputFilename).Run(); err != nil {
			fmt.Fprintf(os.Stderr, s, outputFilename, err)
			os.Exit(1)
		}
	case "darwin":
		if err := exec.Command("open", outputFilename).Run(); err != nil {
			fmt.Fprintf(os.Stderr, s, outputFilename, err)
			os.Exit(1)
		}
	case "windows":
		if err := exec.Command("start", outputFilename).Run(); err != nil {
			fmt.Fprintf(os.Stderr, s, outputFilename, err)
			os.Exit(1)
		}
	default:
		fmt.Printf("open a web browser and navigate to http://localhost%v\n", port)
		f := http.FileServer(http.Dir("."))
		if err := http.ListenAndServe(port, f); err != nil {
			fmt.Fprintf(os.Stderr, "could not locally serve results: %v", err)
			os.Exit(1)
		}
	}
}
