package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	//"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	rootPath = "/"
	userPath = "/user/"
)

type serverContext struct {
	tickets    int64
	dbreads    int32
	computes   int32
	dbmutex    sync.RWMutex
	cachemutex sync.RWMutex
	db         map[string]string
	cache      map[string]string
}

func (s *serverContext) getTicket(user string) (string, int, error) {
	if user == "" || user == "errorc" {
		return "", http.StatusNotFound, fmt.Errorf("getTicket(errorc)")
	}
	if user == "errors" {
		return "", http.StatusInternalServerError, fmt.Errorf("getTicket(errors)")
	}

	// try cache
	t1, errCache := s.cacheRead(user)
	if errCache == nil {
		return t1, http.StatusOK, nil
	}

	// try DB
	t2, errDB := s.dbRead(user)
	if errDB == nil {
		s.cacheWrite(user, t2)
		return t2, http.StatusOK, nil
	}

	// try compute
	t3, errCompute := s.compute(user)
	if errCompute == nil {
		s.dbWrite(user, t3)
		return t3, http.StatusOK, nil
	}

	return "", http.StatusInternalServerError, fmt.Errorf("getTicket() failure: %v", errCompute)
}

func (s *serverContext) cacheRead(user string) (string, error) {
	defer s.cachemutex.RUnlock()
	s.cachemutex.RLock()

	t, found := s.cache[user]
	if found {
		return t, nil
	}

	return "", fmt.Errorf("cacheread not found")
}

func (s *serverContext) cacheWrite(user, ticket string) {
	defer s.cachemutex.Unlock()
	s.cachemutex.Lock()

	s.cache[user] = ticket
}

func (s *serverContext) dbRead(user string) (string, error) {
	defer atomic.AddInt32(&s.dbreads, -1)
	r := atomic.AddInt32(&s.dbreads, 1)
	delay := time.Duration(r) * 200 * time.Millisecond

	log.Printf("dbreads=%d delay=%v", r, delay)

	timeout := 2000 * time.Millisecond
	if delay > timeout {
		time.Sleep(timeout)
		return "", fmt.Errorf("dbread timeout: %v", timeout)
	}

	time.Sleep(delay)

	defer s.dbmutex.RUnlock()
	s.dbmutex.RLock()

	t, found := s.db[user]
	if found {
		return t, nil
	}

	return "", fmt.Errorf("dbread not found")
}

func (s *serverContext) dbWrite(user, ticket string) {
	defer s.dbmutex.Unlock()
	s.dbmutex.Lock()

	s.db[user] = ticket
}

func (s *serverContext) compute(user string) (string, error) {

	defer atomic.AddInt32(&s.computes, -1)
	c := atomic.AddInt32(&s.computes, 1)
	delay := time.Duration(c) * 1000 * time.Millisecond

	log.Printf("computes=%d delay=%v", c, delay)

	timeout := 10000 * time.Millisecond
	if delay > timeout {
		time.Sleep(timeout)
		return "", fmt.Errorf("compute timeout: %v", timeout)
	}

	time.Sleep(delay)

	n := atomic.AddInt64(&s.tickets, 1)
	t := strconv.FormatInt(n, 16)
	return t, nil
}

func main() {

	s := &serverContext{
		db:    map[string]string{},
		cache: map[string]string{},
	}

	http.HandleFunc(rootPath, func(w http.ResponseWriter, r *http.Request) { contextHandle(w, r, s, rootHandler) })
	http.HandleFunc(userPath, func(w http.ResponseWriter, r *http.Request) { contextHandle(w, r, s, userHandler) })

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

func contextHandle(w http.ResponseWriter, r *http.Request, s *serverContext, handler func(http.ResponseWriter, *http.Request, *serverContext)) {
	handler(w, r, s)
}

func rootHandler(w http.ResponseWriter, r *http.Request, s *serverContext) {
	me := "rootHandler"
	msg := fmt.Sprintf("%s: url=%s from=%s", me, r.URL.Path, r.RemoteAddr)
	log.Print(msg)

	code := http.StatusNotFound
	http.Error(w, strconv.Itoa(code)+" - "+http.StatusText(code)+" - "+msg, code)

	//io.WriteString(w, msg)
}

func userHandler(w http.ResponseWriter, r *http.Request, s *serverContext) {
	me := "userHandler"
	msg := fmt.Sprintf("%s: url=%s from=%s", me, r.URL.Path, r.RemoteAddr)
	log.Print(msg)

	user := r.URL.Path[len(userPath):]

	begin := time.Now()

	ticket, code, err := s.getTicket(user)

	elapsed := time.Since(begin)
	e := fmt.Sprintf(" (elapsed=%v) ", elapsed)

	log.Printf("%s: ticket=%s code=%d err=%v"+e, me, ticket, code, err)

	if err != nil {
		http.Error(w, me+": "+strconv.Itoa(code)+" - "+http.StatusText(code)+": "+err.Error()+e, code)
		return
	}

	io.WriteString(w, msg+" ticket="+ticket+e)
}
