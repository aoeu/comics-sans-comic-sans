# comics sans comic sans ![ack](https://raw.github.com/aoeu/comics-sans-comic-sans/master/static/images/bill_icon.png)
___
About:
------
  
This is basic web-comic RSS scraper written with Google Go and HTML, CSS, and Javascript.

__This is meant to be run locally and for personal use and should not be used to redistribute artist's content.__
___
Running:
--------
The program may be run with the default configuration:
`% go run comicsSans.go`

Running the program will:

+ Scrape the RSS feeds specified in _config.json_.
+ Create new files to serve (_index.html_ and _comics.json_).
+ Starts a simple web server (accesible at _http://localhost:8080_ by default).
___
Configuration:
--------------
RSS feeds for web-comics may be added in the config.json file.  
The only field needed per config.json entry is the RSS feed URL, but the _Name_ of the web-comic series may be overloaded as well.
___
Notes:
------
This tool attempts to use a minimal amount of data-munging.  
For that, attempt to build a cleaner feed with Yahoo Pipes or a similar service to pre-process or clean-up RSS feeds.
