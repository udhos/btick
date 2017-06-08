package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	_ "github.com/go-sql-driver/mysql"
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
	mdb        *sql.DB // MySQL
	svcDynamo  *dynamodb.DynamoDB
	realDB     bool
	realCache  bool
}

func (s *serverContext) openDB() {

	dbreal := os.Getenv("DB_REAL")
	s.realDB = dbreal != ""

	if !s.realDB {
		return
	}

	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASS")
	host := os.Getenv("DB_HOST")
	dbname := os.Getenv("DB_NAME")

	msg := fmt.Sprintf("DB_REAL='%s' DB_USER='%s' DB_PASS='%s' DB_HOST='%s' DB_NAME='%s'", dbreal, user, pass, host, dbname)

	if user == "" || pass == "" || host == "" || dbname == "" {
		log.Fatalf("missing parameter: %s", msg)
	}

	log.Print(msg)

	// username:password@protocol(address)/dbname?param=value
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, pass, host, dbname)

	mdb, errDB := sql.Open("mysql", dsn)
	if errDB != nil {
		mdb.Close()
		log.Fatalf("sql open(%s): %v", dsn, errDB)
	}

	s.mdb = mdb
}

func (s *serverContext) openCache() {

	cachereal := os.Getenv("CACHE_REAL")
	s.realCache = cachereal != ""

	if !s.realCache {
		return
	}

	msg := fmt.Sprintf("CACHE_REAL='%s'", cachereal)

	log.Print(msg)

	sess, err := session.NewSession()
	if err != nil {
		log.Fatalf("aws session: %s", err)
	}

	region := os.Getenv("AWS_REGION")
	log.Printf("AWS_REGION=%s", region)
	if region == "" {
		region = "us-east-1"
		log.Printf("missing AWS_REGION, setting region to: %s", region)
	}
	config := aws.NewConfig().WithRegion(region)
	s.svcDynamo = dynamodb.New(sess, config)
	log.Printf("dynamodb cache: region=%s", *config.Region)
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

	log.Printf("cacheread failure: %v", errCache)

	// try DB
	t2, errDB := s.dbRead(user)
	if errDB == nil {
		s.cacheWrite(user, t2)
		return t2, http.StatusOK, nil
	}

	log.Printf("dbread failure: %v", errDB)

	// try compute
	t3, errCompute := s.compute(user)
	if errCompute == nil {
		s.dbWrite(user, t3)
		return t3, http.StatusOK, nil
	}

	return "", http.StatusInternalServerError, fmt.Errorf("getTicket() failure: %v", errCompute)
}

type dynamoItem struct {
	User   string
	Ticket string
}

func (s *serverContext) cacheRead(user string) (string, error) {
	defer s.cachemutex.RUnlock()
	s.cachemutex.RLock()

	if s.realCache {

		input := &dynamodb.GetItemInput{
			Key: map[string]*dynamodb.AttributeValue{
				"User": {
					S: aws.String(user),
				},
			},
			TableName: aws.String("ticket_table"),
		}

		result, errGetItem := s.svcDynamo.GetItem(input)
		if errGetItem != nil {
			return "", fmt.Errorf("dynamoDB cacheread: %v", errGetItem)
		}

		var item dynamoItem
		errUnmarshal := dynamodbattribute.UnmarshalMap(result.Item, &item)
		if errUnmarshal != nil {
			return "", fmt.Errorf("dynamoDB cacheread: unmarshal: %v", errUnmarshal)
		}

		log.Printf("dynamodb cacheread: user=%s ticket=%s", user, item.Ticket)

		return item.Ticket, nil

	}

	t, found := s.cache[user]
	if found {
		return t, nil
	}

	return "", fmt.Errorf("cacheread: not found")
}

func (s *serverContext) cacheWrite(user, ticket string) {
	defer s.cachemutex.Unlock()
	s.cachemutex.Lock()

	if s.realCache {
		log.Printf("dynamoDB cachewrite: FIXME WRITEME")
		return
	}

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

	if s.realDB {

		rows, errQuery := s.mdb.Query("select ticket from ticket_table where user = ?", user)
		if errQuery != nil {
			return "", fmt.Errorf("mysql dbread query: %v", errQuery)
		}

		defer rows.Close()

		rows.Next()
		var t string
		if errScan := rows.Scan(&t); errScan != nil {
			return "", fmt.Errorf("mysql dbread not found: %v", errScan)
		}

		log.Printf("mysql dbread: user=%s ticket=%s", user, t)
		return t, nil
	}

	t, found := s.db[user]
	if found {
		return t, nil
	}

	return "", fmt.Errorf("dbread not found")
}

func (s *serverContext) dbWrite(user, ticket string) {
	defer s.dbmutex.Unlock()
	s.dbmutex.Lock()

	if s.realDB {

		rows, errQuery := s.mdb.Query("insert into ticket_table (user, ticket) values(?,?) on duplicate key update ticket=?", user, ticket, ticket)
		if errQuery != nil {
			log.Printf("mysql dbwrite query: %v", errQuery)
			return
		}

		defer rows.Close()

		log.Printf("mysql dbwrite: user=%s ticket=%s", user, ticket)

		return
	}

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

	s.openDB()
	s.openCache()

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
