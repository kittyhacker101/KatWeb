package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/NYTimes/gziphandler"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Config file structure
type Conf struct {
	IdleTime int `json:"keepAliveTimeout"`
	CachTime int `json:"cachingTimeout"`
	HSTS     struct {
		Run bool `json:"enabled"`
		Sub bool `json:"includeSubDomains"`
		Pre bool `json:"preload"`
	} `json:"hsts"`
	Secure bool `json:"https"`
	BSniff bool `json:"nosniff"`
	IFrame bool `json:"sameorigin"`
	Zip    bool `json:"gzip"`
	Dyn    bool `json:"dynamicServing"`
}

// Redirect you to the secure version.
func redirectToHttps(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
	fmt.Println(r.RemoteAddr + " - HTTPS Redirect")
}

// Check if path exists for domain, and use it instead of default if it does.
func detectPath(p string) string {
	_, err := os.Stat(p)
	if err != nil {
		return "html/"
	} else {
		if p == "ssl/" {
			return "error/"
		} else {
			return p
		}
	}
}

func main() {
	// Load and parse config files
	var conf Conf
	fmt.Println("Loading config files...")
	data, _ := ioutil.ReadFile("./conf.json")
	json.Unmarshal(data, &conf)
	fmt.Println("Loading server...")

	// We must use the UTC format when using .Format(http.TimeFormat) on the time.
	location, _ := time.LoadLocation("UTC")

	// This handles all web requests
	mainHandle := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Check path and file info
		var path string = detectPath(r.Host + "/")
		finfo, err := os.Stat(path + r.URL.Path[1:])

		// Add important headers
		w.Header().Add("Server", "KatWeb Alpha")
		w.Header().Add("Keep-Alive", "timeout="+strconv.Itoa(conf.IdleTime))
		if conf.CachTime != 0 {
			w.Header().Set("Cache-Control", "max-age="+strconv.Itoa(3600*conf.CachTime)+", public, stale-while-revalidate=3600")
			w.Header().Set("Expires", time.Now().In(location).Add(time.Duration(conf.CachTime)*time.Hour).Format(http.TimeFormat))
		}
		if conf.HSTS.Run {
			if conf.HSTS.Sub {
				if conf.HSTS.Pre {
					w.Header().Add("Strict-Transport-Security", "max-age=31536000;includeSubDomains;preload")
				} else {
					w.Header().Add("Strict-Transport-Security", "max-age=31536000;includeSubDomains")
				}
			} else {
				// Preload requires includeSubDomains for some reason, idk why.
				w.Header().Add("Strict-Transport-Security", "max-age=31536000")
			}
		}
		if conf.BSniff {
			w.Header().Add("X-Content-Type-Options", "nosniff")
		}
		if conf.IFrame {
			w.Header().Add("X-Frame-Options", "sameorigin")
		}
		// Check if file exists, and if it does then add modification timestamp. Then send file.
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Last-Modified", time.Now().In(location).Format(http.TimeFormat))
			fmt.Println(r.RemoteAddr + " - 404 Error")
			http.ServeFile(w, r, "error/NotFound.html")
		} else {
			w.Header().Set("Last-Modified", finfo.ModTime().In(location).Format(http.TimeFormat))
			fmt.Println(r.RemoteAddr + " - " + r.Host + r.URL.Path)
			http.ServeFile(w, r, path+r.URL.Path[1:])
		}
	})

	// HTTP Compression!!!
	var handleGz http.Handler
	if conf.Zip {
		handleGz = gziphandler.GzipHandler(mainHandle)
	} else {
		handleGz = mainHandle
	}

	// Config for HTTPS, basicly making things a lil more secure
	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		NextProtos:               []string{"h2", "http/1.1"},
	}
	// Config for HTTPS Server
	srv := &http.Server{
		Addr:         ":443",
		Handler:      handleGz,
		TLSConfig:    cfg,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTime) * time.Second,
	}
	// Config for HTTP Server, redirects to HTTPS
	srvh := &http.Server{
		Addr:         ":80",
		Handler:      http.HandlerFunc(redirectToHttps),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTime) * time.Second,
	}
	// Secondary Config for HTTP Server.
	srvn := &http.Server{
		Addr:         ":80",
		Handler:      handleGz,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTime) * time.Second,
	}

	// This code actually starts the servers.
	fmt.Println("KatWeb HTTP Server Started.")
	if conf.Secure {
		// We use a Goroutine because the HTTP and HTTPS servers need to run at the same time, because 99% of browser default to HTTP.
		// If browsers defaulted to HTTPS, this wouldn't be needed.
		if conf.HSTS.Run {
			// HTTP Server which redirects to HTTPS
			go srvh.ListenAndServe()
		} else {
			// Serves the same content as HTTPS, but unencrypted.
			go srvn.ListenAndServe()
		}
		srv.ListenAndServeTLS("ssl/server.crt", "ssl/server.key")
	} else {
		srvn.ListenAndServe()
	}
}