package main

import (
	"database/sql"
	"fmt"
	"net/url"
)

type GemThreadMessage struct {
	id         int64
	url        string
	author     string
	title      string
	dt_created string
	summary    string
}

func (msg GemThreadMessage) String() string {
	rstr := "=> " + msg.url + " " + msg.author + " — " + msg.title + "\r\n"
	rstr += msg.dt_created
	if len(msg.summary) > 0 {
		rstr += " - " + msg.summary
	}
	rstr += "\r\n"
	return rstr
}

func (msg GemThreadMessage) FullString() string {
	rstr := fmt.Sprintf("Message %d\r\n", msg.id)
	rstr += "=> " + server_url() + fmt.Sprintf("/messages/%d MessageID: %d\r\n", msg.id, msg.id)
	rstr += msg.String()
	return rstr
}

func (msg GemThreadMessage) TextString() string {
	rstr := "```\r\n"
	rstr += fmt.Sprintf("Message Source URL: %s\r\n", msg.url)
	rstr += fmt.Sprintf("%s — %s\r\n", msg.author, msg.title)
	rstr += msg.dt_created
	if len(msg.summary) > 0 {
		rstr += " - " + msg.summary
	}
	rstr += "\r\n"
	rstr += "```\r\n"
	return rstr
}

func (msg GemThreadMessage) InstancesString(db *sql.DB) string {
	str := fmt.Sprintf("## Message ID %d\r\n", msg.id)
	str += fmt.Sprintf("=> %s/messages/%d MessageID: %d\r\n", server_url(), msg.id, msg.id)
	str += msg.TextString()
	str += fmt.Sprintf("=> %s/messages/%d/update?%s Refetch and update this message\r\n", server_url(), msg.id, url.QueryEscape(msg.url))
	thr_init, err := db_find_thread_by_originating_message_id(db, msg.id)
	if err != nil {
		str += fmt.Sprintf("Error when searching for a thread originated by this message: %s\r\n", err.Error())
	} else if thr_init.id > 0 {
		str += fmt.Sprintf("### Message %d initiates thread ID %d\r\n", msg.id, thr_init.id)
		str += thr_init.String()
	}
	thr_resp, err := db_find_threads_by_responding_message_id(db, msg.id)
	if err != nil {
		str += fmt.Sprintf("Error when searching for threads that this message responds to: %s\r\n", err.Error())
	} else if len(thr_resp) > 0 {
		str += fmt.Sprintf("### Message %d is a response to the following threads:\r\n", msg.id)
		for _, thr_r := range thr_resp {
			str += thr_r.String()
		}
	}
	return str
}

type GemThreadThread struct {
	id         int64
	author     string
	title      string
	dt_created string
	dt_updated string
}

func (thr GemThreadThread) String() string {
	str := fmt.Sprintf("=> %s/threads/%d %s: %s — %s\r\n", server_url(), thr.id, thr.dt_created, thr.author, thr.title)
	if len(thr.dt_updated) > 0 {
		str += fmt.Sprintf("* Last response on %s\r\n", thr.dt_updated)
	} else {
		str += "* No responses\r\n"
	}
	return str
}
