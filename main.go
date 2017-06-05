package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	//"runtime"
	//"time"
	"strconv"
)

func main() {

	http.HandleFunc("/", rootHandler)      // root handler
	http.HandleFunc("/user/", userHandler) // user handler

	//registerStatic("/www/", currDir)

	addr := ":8080"

	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	log.Printf("serving on port TCP %s", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Panicf("ListenAndServe: %s: %s", addr, err)
	}
}

/*
type staticHandler struct {
	innerHandler http.Handler
}

func registerStatic(path, dir string) {
	http.Handle(path, staticHandler{http.StripPrefix(path, http.FileServer(http.Dir(dir)))})
	log.Printf("registering static directory %s as www path %s", dir, path)
}

func (handler staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("staticHandler.ServeHTTP url=%s from=%s", r.URL.Path, r.RemoteAddr)
	handler.innerHandler.ServeHTTP(w, r)
}
*/

func rootHandler(w http.ResponseWriter, r *http.Request) {
	me := "rootHandler"
	msg := fmt.Sprintf("%s: url=%s from=%s", me, r.URL.Path, r.RemoteAddr)
	log.Printf(msg)

	code := http.StatusNotFound
	http.Error(w, strconv.Itoa(code)+" - "+http.StatusText(code)+" - "+msg, code)

	//io.WriteString(w, msg)
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	me := "userHandler"
	msg := fmt.Sprintf("%s: url=%s from=%s", me, r.URL.Path, r.RemoteAddr)
	log.Printf(msg)

	user := r.URL.Path[len("/user/"):]

	if user == "" || user == "errorc" {
		code := http.StatusNotFound
		http.Error(w, me+": "+strconv.Itoa(code)+" - "+http.StatusText(code), code)
		return
	}

	ticket, err := getTicket(user)
	if err != nil {
		code := http.StatusInternalServerError
		http.Error(w, me+": "+strconv.Itoa(code)+" - "+http.StatusText(code)+": "+err.Error(), code)
		return
	}

	io.WriteString(w, msg+" ticket="+ticket)
}

func getTicket(user string) (string, error) {
	if user == "errors" {
		return "", fmt.Errorf("getTicket(errors)")
	}
	return user, nil
}
