package main

import (
	"io"
	"bytes"
	"golang.org/x/net/html/charset"
	"golang.org/x/net/html"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"flag"
	"github.com/PuerkitoBio/goquery"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type RSS struct {
	Version string `xml:"version,attr"`
	// Using shorthand of just "Channel" doesn't work here, e.g.
	// Channel `xml:"channel"`
	Channel Channel `xml:"channel"`
}

type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	LastBuildDate string `xml:"lastBuildDate"`
	Items         []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Guid        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// Data structure for representing a single web comic image and related data.
type Comic struct {
	Title        string
	Link         string
	ImageUrl     string
	ImageComment string
	Date         string
	UnixDate     int64
	PubMsg       string
}

// Sets questionable data member values to preferred defaults.
func (c *Comic) sanitize() {
	if (*c).Title == "." { // Hack: yahoo pipes will not allow blank title.
		(*c).Title = ""
	}
	// Do not allow repition of title in image comment.
	if (*c).ImageComment == (*c).Title {
		(*c).ImageComment = ""
	}
}

// Data structure for multiple comics published by a single site or author.
type ComicSeries struct {
	SeriesTitle string
	SiteUrl     string
	Description string
	Index       int // For the front-end to keep track of Comic to display.
	Comics      []Comic
}

// Sets questionable data member values to preferred defaults.
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

// Data structure for meta data relevant to obtaining a Comic or ComicSeries.
type ComicMetaData struct {
	Url        string
	ImgAttrs   map[string]string
	ImgComment string
	Name       string
	RSSFeed    *RSS
}

// Requests data from a URL and stores response data.
func (c *ComicMetaData) downloadUrl(wg *sync.WaitGroup) {
	resp, err := http.Get(c.Url)
	if err != nil {
		log.Println("Did not receive HTTP GET response: ", err)
	}
	if resp.StatusCode != 200 {
		log.Printf("Bad HTTP Response %d for %s\n", resp.StatusCode, c.Url)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
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
		log.Println("Problem unmarshalling XML for", c.Url, err)
	}
	c.RSSFeed = rss
	wg.Done()
}

// Calculates, formats, and returns time information about an RSS <pubDate>.
func getDateData(pubDate string) (
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

// Creates an English sentence stating how much time has elapsed
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

// Obtains Comic data from a single RSS <item> node.
func getComic(item Item, commentAttrName string) (c Comic, err error) {
	c = Comic{Title: item.Title, Link: item.Link}
	// TODO: Why ignore error?
	c.Date, c.UnixDate, c.PubMsg, _ = getDateData(item.PubDate)
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
		if c.ImageUrl == "" {
			c.ImageUrl, _ = img.Attr("src")
			c.ImageComment, _ = img.Attr(commentAttrName)
		} else {
			return // Keep going until we set any image.
		}
	})
	c.sanitize()
	return
}

// Obtains an entire series of Comics from an RSS feed.
func getComicSeries(feed *RSS, commentAttrName string) (ComicSeries, error) {
	if commentAttrName == "" {
		commentAttrName = "alt"
	}
	series := new(ComicSeries)
	series.SeriesTitle = feed.Channel.Title
	series.SiteUrl = feed.Channel.Link
	series.Description = feed.Channel.Description
	series.Index = 0
	lastBuildDate := feed.Channel.LastBuildDate
	var comics []Comic
	for _, item := range feed.Channel.Items {
		comic, err := getComic(item, commentAttrName)
		if err != nil {
			log.Println(err)
			continue
		}
		if comic.UnixDate < 0 { // Hack: Some comics don't have pubDate on items.
			comic.Date, comic.UnixDate, comic.PubMsg, _ = getDateData(lastBuildDate)
		}
		comics = append(comics, comic)
	}
	series.Comics = comics
	series.sanitize()
	return *series, nil
}

// Obtains ComicSeries data for each RSS feed (represented by meta data).
func parseFeeds(metaData []*ComicMetaData) (comics []ComicSeries) {
	for _, md := range metaData {
		var comic ComicSeries
		comic, err := getComicSeries(md.RSSFeed, md.ImgComment)
		if md.Name != "" {
			comic.SeriesTitle = md.Name
		}
		if len(comic.Comics) != 0 {
			comics = append(comics, comic)
		} else {
			log.Printf("Error with %s, no actual comics in feed.\n", comic.SiteUrl)
			if err != nil {
				log.Printf("Received error %s for %s", err, comic.SiteUrl)
			}
		}
	}
	return
}

// Helper method for concurrently domnloading RSS feed data from a URL.
func downloadFeeds(config []*ComicMetaData) {
	var wg sync.WaitGroup
	for _, c := range config {
		wg.Add(1)
		go c.downloadUrl(&wg)
	}
	wg.Wait()
}

// Parses a configuration file specifying RSS feeds and
// constructs meta data to represent them.
func parseConfig(configFileName string) []*ComicMetaData {
	data, _ := ioutil.ReadFile(configFileName)
	var config []ComicMetaData
	json.Unmarshal(data, &config)
	var ptrs []*ComicMetaData
	for i, _ := range config {
		ptrs = append(ptrs, &config[i])
	}
	return ptrs
}

// A basic quicksort algorithm for sorting a ComicSeries by
// chronological order for the publishing date.
func quickSort(series []ComicSeries) []ComicSeries {
	length := len(series)
	if length < 2 {
		return series
	}
	defer func() {
		if err := recover(); err != nil {
			log.Println("quickSort failed: ", err)
		}
	}()
	pivot := (length / 2) - 1
	var lesser, greater []ComicSeries
	for index, _ := range series {
		if index == pivot {
			continue
		}
		time1 := series[index].Comics[0].UnixDate
		time2 := series[pivot].Comics[0].UnixDate
		if time1 <= time2 {
			lesser = append(lesser, series[index])
		} else {
			greater = append(greater, series[index])
		}
	}
	lesserLen := len(lesser)
	result := make([]ComicSeries, lesserLen+1+len(greater))
	copy(result, quickSort(lesser))
	copy(result[lesserLen:], series[pivot:pivot+1])
	copy(result[lesserLen+1:], quickSort(greater))
	return result
}

// Reverses the sort order of a ComicSeries.
func reverse(series []ComicSeries) []ComicSeries {
	j := len(series) - 1
	result := make([]ComicSeries, len(series))
	for i, _ := range series {
		result[j-i] = series[i]
	}
	return result
}

// Functions for use in the HTML template.
func incr(n int) string { return fmt.Sprintf("%d", n+1) }
func decr(n int) string { return fmt.Sprintf("%d", n-1) }

// For use in main. Exit fast and loudly if something goes wrong.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

var port string 
func main() {
	flag.StringVar(&port, "port", ":8080", "The port to serve the page on.")
	flag.Parse()
	// Sanity check and construct template first.
	tmplText, err := ioutil.ReadFile("static/template.html")
	check(err)
	tmplString := string(tmplText)
	funcMap := template.FuncMap{"incr": incr, "decr": decr}
	tmpl, err := template.New("comics").Funcs(funcMap).Parse(tmplString)
	check(err)

	// Set up configuration.
	metaData := parseConfig("config.json")

	// Download and parse feeds.
	downloadFeeds(metaData)
	comics := parseFeeds(metaData)

	// Sort and output results.
	outputFile, err := os.Create("index.html")
	check(err)
	comics = reverse(quickSort(comics))
	err = tmpl.Execute(outputFile, comics)
	check(err)

	// Output data into JSON file for use by the front end.
	jsonData, err := json.Marshal(comics)
	check(err)
	err = ioutil.WriteFile("comics.json", jsonData, 0644)
	check(err)

	// Serve the front-end and generated data.
	panic(http.ListenAndServe(port, http.FileServer(http.Dir("."))))
}
