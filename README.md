A basic web-comic RSS scraper written with Google Go and HTML5.

This tool is meant to be run locally and for personal use and should not be used to redistribute artist's content.

The program may be run with the default configuration:
    % go run comicsSans.go

This will start a simple web server, allowing access to the view at http://localhost:8080 by default.


RSS feeds for web-comics may be added in the config.json file.
The only field needed per config.json entry is the RSS feed URL,
but the Name of the web-comic may be overloaded as well.

This tool attempts to use a minimal amount of data-munging.
For that, attempt to build a cleaner feed with Yahoo Pipes or a similar service to pre-process or clean-up RSS feeds.
