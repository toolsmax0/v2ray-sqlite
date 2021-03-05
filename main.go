package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/go-yaml/yaml"
	"github.com/liushuochen/gotable"
	_ "github.com/mattn/go-sqlite3"
	"github.com/v2fly/v2ray-core/app/stats/command"
	"google.golang.org/grpc"
)

//Stat  a single record of traffic
type Stat struct {
	User  string
	Type  string
	value int64
}

func readableSize(n int64) (s string) {
	f := float64(n)
	p := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f > 1024 {
		f /= 1024
		i++
	}
	s = strconv.FormatFloat(f, 'f', 2, 64) + p[i]
	return
}

func queryStats(c command.StatsServiceClient) (stats []Stat, err error) {
	resp, err := c.QueryStats(context.Background(), &command.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  true,
	})
	if err != nil {
		return
	}
	stats = make([]Stat, len(resp.GetStat()))
	for i, s := range resp.GetStat() {
		slice := strings.Split(s.Name, ">>>")
		stats[i].User = slice[1]
		stats[i].Type = slice[3]
		stats[i].value = s.Value
	}
	return
}
func queryServerStats(addr string) (stats []Stat, err error) {
	cmdConn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return
	}
	defer func() { _ = cmdConn.Close() }()
	statsClient := command.NewStatsServiceClient(cmdConn)
	stats, err = queryStats(statsClient)
	if err != nil {
		return
	}
	return
}
func writeToDB(database string, table string, stats []Stat, reset bool) (err error) {
	db, err := sql.Open("sqlite3", database)
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer db.Close()
	SUMU := Stat{"SUM", "uplink", 0}
	SUMD := Stat{"SUM", "downlink", 0}
	head := []string{"User", "Flow"}
	tab, err := gotable.CreateTable(head)
	if err != nil {
		log.Fatalln(err)
		return
	}
	eg, err := db.Begin()
	if err != nil {
		log.Fatalln(err)
		return
	}

	query, err := eg.Prepare(fmt.Sprintf("select value from %s where user = ? and type = ? ", table))
	if err != nil {
		log.Fatalln(err)
		return
	}

	update, err := eg.Prepare(fmt.Sprintf("insert or replace into %s values( ? , ? , ? )", table))
	if err != nil {
		log.Fatalln(err)
		return
	}

	for _, stat := range stats {
		rows, err := query.Query(stat.User, stat.Type)
		if err != nil {
			log.Fatalln(err)
			break
		}
		for rows.Next() {
			var value int64
			err = rows.Scan(&value)
			if err != nil {
				log.Fatalln(err)
				break
			}
			stat.value += value
		}
		rows.Close()
		switch stat.Type {
		case "uplink":
			SUMU.value += stat.value
		case "downlink":
			SUMD.value += stat.value
		}

		_, err = update.Exec(stat.User, stat.Type, stat.value)
		if err != nil {
			log.Fatalln(err)
		}
		rec := gotable.CreateEmptyValueMap()
		rec["User"] = gotable.CreateValue(fmt.Sprintf("%s->%s", stat.User, stat.Type))
		rec["Flow"] = gotable.CreateValue(readableSize(stat.value))
		tab.AddValue(rec)
	}
	_, err = update.Exec(SUMU.User, SUMU.Type, SUMU.value)
	if err != nil {
		log.Fatalln(err)
	}
	_, err = update.Exec(SUMD.User, SUMD.Type, SUMD.value)
	if err != nil {
		log.Fatalln(err)
	}
	query.Close()
	update.Close()
	if reset {
		_, err = eg.Exec(fmt.Sprintf("delete from %s", table))
		if err != nil {
			log.Fatalln(err)
			return
		}
	}
	eg.Commit()
	rec := gotable.CreateEmptyValueMap()
	rec["User"] = gotable.CreateValue(fmt.Sprintf("%s->%s", SUMU.User, SUMU.Type))
	rec["Flow"] = gotable.CreateValue(readableSize(SUMU.value))
	tab.AddValue(rec)
	rec = gotable.CreateEmptyValueMap()
	rec["User"] = gotable.CreateValue(fmt.Sprintf("%s->%s", SUMD.User, SUMD.Type))
	rec["Flow"] = gotable.CreateValue(readableSize(SUMD.value))
	tab.AddValue(rec)
	rec = gotable.CreateEmptyValueMap()
	rec["User"] = gotable.CreateValue(fmt.Sprint("SUM"))
	rec["Flow"] = gotable.CreateValue(readableSize(SUMD.value + SUMU.value))
	tab.AddValue(rec)
	tab.PrintTable()
	return
}

//T config struct for YAML
type T struct {
	Server   string `yaml:"server"`
	Database string `yaml:"database"`
	Table    string `yaml:"table"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)

	configPath := flag.String("config", "./config.yml", "path to config file")
	reset := flag.Bool("reset", false, "whether to reset the records in the database")
	flag.Parse()
	conf, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatalln(err)
	}
	m := T{}
	err = yaml.Unmarshal(conf, &m)
	if err != nil {
		log.Fatalln(err)
	}
	server := m.Server
	stats, err := queryServerStats(server)
	if err != nil {
		log.Println("fail to QueryServerStats", server, err)
	}
	if len(stats) <= 0 {
		return
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].User == stats[j].User {
			return stats[i].Type > stats[j].Type
		}
		return stats[i].User < stats[j].User
	})
	err = writeToDB(m.Database, m.Table, stats, *reset)
	if err != nil {
		log.Fatalln(err)
	}
}
