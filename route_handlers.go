package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"
)

func handle_help(fd io.ReadWriteCloser) {

	// Fail if file does not exist or perms aren't right
	info, err := os.Stat(help_path())
	if os.IsNotExist(err) || os.IsPermission(err) {
		write_response(fd, 51, "help file not found")
		return
	} else if err != nil {
		write_response(fd, 40, "temporary failure for help file")
		return
	} else if uint64(info.Mode().Perm())&0444 != 0444 {
		write_response(fd, 51, "help file not found")
		return
	}

	help_data, err := ioutil.ReadFile(help_path())
	if err != nil {
		write_response(fd, 50, "error reading help file")
		return
	}

	help_string := string(help_data)

	type server_info struct {
		ServerURL string
	}

	sinfo := server_info{
		ServerURL: server_url(),
	}

	t, err := template.New("help").Parse(help_string)
	if err != nil {
		write_response(fd, 50, "error while parsing help file template: "+err.Error())
		return
	}

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, sinfo); err != nil {
		write_response(fd, 50, "error while compiling help file: "+err.Error())
		return
	}

	write_response(fd, 20, tpl.String())

	return
}

// handle_threads handles URLs of the following forms:
// => gemini://hostname.xyz/gemthread/threads
// => gemini://hostname.xyz/gemthread/threads?start=0&count=100&sort=CREATE
// => gemini://hostname.xyz/gemthread/threads/new?<URL_ENCODED_URL>
// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>
// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>/respond?<URL_ENCODED_URL>
func handle_threads(fd io.ReadWriteCloser, db *sql.DB, pathcomps []string, query_string string) {

	if len(pathcomps) == 1 {
		// URL is gemini://hostname.xyz/gemthread/threads

		start := 0
		count := 100
		ascending := false       // order is descending by default
		by_date_created := false // sort by date updated by default

		if len(query_string) > 0 {
			query_map, err := parse_query_string_to_map(query_string)
			if err != nil {
				write_response(fd, 50, "error parsing query string: "+err.Error())
				return
			}

			// start : default is 0
			// count : default is 100
			// sort : "update" or "UPDATE" or "U", or "create" or "CREATE" or "creation", default is
			// order : "a" or "ascending", or "d" or "descending"

			q_start, ok := query_map["start"]
			if ok {
				start, err = strconv.Atoi(q_start)
				if err != nil {
					write_response(fd, 50, fmt.Sprintf("error parsing 'start' parameter '%s': %s", q_start, err.Error()))
					return
				}
			}

			q_count, ok := query_map["count"]
			if ok {
				count, err = strconv.Atoi(q_count)
				if err != nil {
					write_response(fd, 50, fmt.Sprintf("error parsing 'count' parameter '%s': %s", q_count, err.Error()))
					return
				}
			}

			q_order := query_map["order"]
			if len(q_order) > 0 {
				if strings.HasPrefix(strings.ToUpper(q_order), "A") {
					ascending = true
				} else if strings.HasPrefix(strings.ToUpper(q_order), "D") {
					ascending = false
				} else {
					write_response(fd, 50, "error in 'order' parameter: "+q_order)
					return
				}
			}

			q_sort := query_map["sort"]
			if len(q_sort) > 0 {
				if strings.HasPrefix(strings.ToUpper(q_sort), "C") {
					by_date_created = true
				} else if strings.HasPrefix(strings.ToUpper(q_sort), "U") {
					by_date_created = false
				} else {
					write_response(fd, 50, "error in 'sort' parameter: "+q_sort)
					return
				}
			}

		}

		threads, err := db_list_threads(db, start, count, ascending, by_date_created)

		if err != nil {
			write_response(fd, 50, err.Error())
			return
		}

		rtxt := ""

		for _, thread := range threads {
			rtxt += thread.String()
		}

		rtxt += fmt.Sprintf("=> %s/threads/new Create a new thread\r\n", server_url())

		write_response(fd, 20, rtxt)
		return
	}

	if pathcomps[1] == "new" {
		// URL is gemini://hostname.xyz/gemthread/threads/new?<URL_ENCODED_URL>

		if len(query_string) == 0 {
			write_response(fd, 10, "Please enter the URL for the new thread's initial message")
			return
		}

		tgt_url, err := url.QueryUnescape(query_string)
		if err != nil || tgt_url == "" {
			write_response(fd, 59, "unable to unescape query string: "+query_string)
			return
		}

		if !strings.HasPrefix(tgt_url, "gemini://") {
			write_response(fd, 50, "only gemini:// URLs may be added to a GemThreads server")
			return
		}

		result, err := retrieve(tgt_url)
		if err != nil {
			write_response(fd, 50, "unable to retrieve "+tgt_url+": "+err.Error())
			return
		}

		msg, is_allowed, err := parse_post(tgt_url, result)
		if err != nil {
			write_response(fd, 50, "unable to parse "+tgt_url+" contents: "+err.Error())
			return
		}

		if !is_allowed {
			write_response(fd, 50, "PROHIBITED: the requested page contains \"GemThread.Prohibit\"")
			return
		}

		thr_id, err := db_create_new_thread(db, msg)

		if err != nil {
			if thr_id < 0 {
				write_response(fd, 50, err.Error())
				return
			}
			// Otherwise, this is a pre-existing thread, and we simply continue.
		}

		write_response(fd, 30, fmt.Sprintf("%s/threads/%d", server_url(), thr_id))

		return
	}

	// If we have reached this point, the only other thing that pathcomps[1]
	// is permitted to be is a numeric thread ID. URLs will be of the form:
	// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>
	// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>/respond?<URL_ENCODED_URL>

	thr_id, err := strconv.Atoi(pathcomps[1])
	if err != nil {
		write_response(fd, 59, "invalid or malformed thread ID "+pathcomps[1])
		return
	}

	if len(pathcomps) == 2 {

		// URL is of the form:
		// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>
		// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>?order=descending
		// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>?order=ascending

		// So, the user wants to view the thread

		ascending := true // order is ascending by default

		if len(query_string) > 0 {
			query_map, err := parse_query_string_to_map(query_string)
			if err != nil {
				write_response(fd, 50, "error parsing query string: "+err.Error())
				return
			}

			q_order := query_map["order"]
			if len(q_order) > 0 {
				if strings.HasPrefix(strings.ToUpper(q_order), "A") {
					ascending = true
				} else if strings.HasPrefix(strings.ToUpper(q_order), "D") {
					ascending = false
				} else {
					write_response(fd, 50, "error in 'order' parameter: "+q_order)
					return
				}
			}
		}

		thr, err := db_find_thread_by_id(db, int64(thr_id))
		if err != nil {
			write_response(fd, 50, "error while retrieving thread: "+err.Error())
			return
		}

		msgs, err := db_find_messages_for_thread(db, int64(thr_id), ascending)
		if err != nil {
			write_response(fd, 50, "error while finding messages for thread: "+err.Error())
			return
		}

		rstr := fmt.Sprintf("# %s — %s\r\n", thr.author, thr.title)
		for _, msg := range msgs {
			rstr += msg.String()
		}
		rstr += fmt.Sprintf("=> %s/threads/%d/respond Add a response to this thread\r\n", server_url(), thr_id)
		rstr += fmt.Sprintf("=> %s/threads/ See all threads\r\n", server_url())

		write_response(fd, 20, rstr)

		return
	}

	// URL should be of the form:
	// => gemini://hostname.xyz/gemthread/threads/<THREAD_ID>/respond?<URL_ENCODED_URL>
	if pathcomps[2] == "respond" {

		if query_string == "" {
			write_response(fd, 10, "Please enter the URL for the response message")
			return
		}

		tgt_url, err := url.QueryUnescape(query_string)
		if err != nil || tgt_url == "" {
			write_response(fd, 59, "unable to unescape query string "+query_string)
			return
		}

		result, err := retrieve(tgt_url)
		if err != nil {
			write_response(fd, 50, "unable to retrieve "+tgt_url+": "+err.Error())
		}

		msg, is_allowed, err := parse_post(tgt_url, result)
		if err != nil {
			write_response(fd, 50, "unable to parse "+tgt_url+" contents: "+err.Error())
			return
		}

		if !is_allowed {
			write_response(fd, 50, "PROHIBITED: the requested page contains \"GemThread.Prohibit\"")
			return
		}

		msg.id, err = db_insert_response_message(db, int64(thr_id), msg)
		if err != nil {
			write_response(fd, 50, "unable to insert message: "+err.Error())
			return
		}

		write_response(fd, 30, fmt.Sprintf("%s/threads/%d", server_url(), thr_id))

		return
	}

	write_response(fd, 59, "invalid or malformed thread action "+pathcomps[2])
	return
}

// Handle requests of the form:
// => gemini://hostname.xyz/gemthread/messages/<MESSAGE_ID>
// => gemini://hostname.xyz/gemthread/messages/<MESSAGE_ID>/update
func handle_messages(fd io.ReadWriteCloser, db *sql.DB, pathcomps []string, query_string string) {

	if len(pathcomps) == 1 {
		// URL is gemini://hostname.xyz/gemthread/messages

		start := 0
		count := 100
		ascending := false // order is descending by default

		if len(query_string) > 0 {
			query_map, err := parse_query_string_to_map(query_string)
			if err != nil {
				write_response(fd, 50, "error parsing query string: "+err.Error())
				return
			}

			// start : default is 0
			// count : default is 100
			// order : "a" or "ascending", or "d" or "descending"

			q_start, ok := query_map["start"]
			if ok {
				start, err = strconv.Atoi(q_start)
				if err != nil {
					write_response(fd, 50, fmt.Sprintf("error parsing 'start' parameter '%s': %s", q_start, err.Error()))
					return
				}
			}

			q_count, ok := query_map["count"]
			if ok {
				count, err = strconv.Atoi(q_count)
				if err != nil {
					write_response(fd, 50, fmt.Sprintf("error parsing 'count' parameter '%s': %s", q_count, err.Error()))
					return
				}
			}

			q_order := query_map["order"]
			if len(q_order) > 0 {
				if strings.HasPrefix(strings.ToUpper(q_order), "A") {
					ascending = true
				} else if strings.HasPrefix(strings.ToUpper(q_order), "D") {
					ascending = false
				} else {
					write_response(fd, 50, "error in 'order' parameter: "+q_order)
					return
				}
			}

		}

		msgs, err := db_list_messages(db, start, count, ascending)

		if err != nil {
			write_response(fd, 50, err.Error())
			return
		}

		rstr := ""

		for _, msg := range msgs {
			rstr += "=> " + server_url() + fmt.Sprintf("/messages/%d MessageID: %d\r\n", msg.id, msg.id)
			rstr += msg.TextString()
		}

		write_response(fd, 20, rstr)
		return
	}

	// If we are here, URL is either:
	// => gemini://twistedcarrot.com/gemthread/messages/<MESSAGE_ID>
	// or
	// => gemini://twistedcarrot.com/gemthread/messages/<MESSAGE_ID>/update?<URL_ENCODED_URL>

	msg_id, err := strconv.Atoi(pathcomps[1])
	if err != nil {
		write_response(fd, 59, "invalid or malformed message ID "+pathcomps[1])
		return
	}

	if len(pathcomps) == 2 {
		// => gemini://twistedcarrot.com/gemthread/messages/<MESSAGE_ID>
		msg, err := db_find_message_by_id(db, int64(msg_id))
		if err != nil {
			write_response(fd, 51, fmt.Sprintf("error when attempting to find message with ID %d: %s", msg_id, err.Error()))
			return
		}
		rstr := msg.InstancesString(db)
		write_response(fd, 20, rstr)
		return
	}

	if len(pathcomps) == 3 {
		// => gemini://twistedcarrot.com/gemthread/messages/<MESSAGE_ID>/update?<URL_ENCODED_URL>

		if query_string == "" {
			write_response(fd, 10, "Please enter the URL for the updated message")
			return
		}

		tgt_url, err := url.QueryUnescape(query_string)
		if err != nil || tgt_url == "" {
			write_response(fd, 59, "unable to unescape query string: "+query_string)
			return
		}

		if !strings.HasPrefix(tgt_url, "gemini://") {
			write_response(fd, 50, "only URLs beginning with gemini:// may be added to a gemthreads server")
			return
		}

		saved_msg, err := db_find_message_by_id(db, int64(msg_id))
		if err != nil {
			write_response(fd, 51, fmt.Sprintf("error when attempting to find message with ID %d: %s", msg_id, err.Error()))
			return
		}

		if saved_msg.url != tgt_url {
			write_response(fd, 50, "URL passed as query parameter does not match stored message URL")
			return
		}

		result, err := retrieve(tgt_url)
		if err != nil {
			write_response(fd, 50, "unable to retrieve "+tgt_url+": "+err.Error())
			return
		}

		retrieved_msg, is_allowed, err := parse_post(tgt_url, result)
		if err != nil {
			write_response(fd, 50, "unable to parse "+tgt_url+" contents: "+err.Error())
			return
		}

		if !is_allowed {

			_, err = db_delete_message(db, saved_msg)
			if err != nil {
				write_response(fd, 50, "unable to delete message: "+err.Error())
				return
			}

			write_response(fd, 20, fmt.Sprintf("Removed message with ID %d from database in response to \"GemThread.Prohibit\" line.", saved_msg.id))
			return
		}

		saved_msg.author = retrieved_msg.author
		saved_msg.title = retrieved_msg.title
		saved_msg.summary = retrieved_msg.summary
		_, err = db_update_message(db, saved_msg, nil)
		if err != nil {
			write_response(fd, 50, "unable to update message: "+err.Error())
			return
		}

		write_response(fd, 20, saved_msg.FullString())
		return

	}

	write_response(fd, 51, "invalid message URL")
	return

}

// Handle requests of the form:
// => gemini://twistedcarrot.com/gemthread/search?<URL_ENCODED_URL_PATH>
func handle_search(fd io.ReadWriteCloser, db *sql.DB, pathcomps []string, query_string string) {

	if query_string == "" {
		write_response(fd, 10, "Please enter the URL or partial URL for which to search")
		return
	}

	tgt_url, err := url.QueryUnescape(query_string)
	if err != nil || tgt_url == "" {
		write_response(fd, 59, "unable to unescape query string "+query_string)
		return
	}

	msgs, err := db_find_message_by_url(db, tgt_url, true)
	if err != nil {
		write_response(fd, 50, "error during query: "+err.Error())
		return
	}

	rstr := "# Search results for " + tgt_url + "\r\n\r\n"

	for _, msg := range msgs {
		rstr += msg.InstancesString(db)
	}

	write_response(fd, 20, rstr)

	return
}

func handle_request(fd io.ReadWriteCloser, db *sql.DB) {

	defer fd.Close()

	request_bytes, err := read_request_bytes(fd)

	if err != nil {
		write_response(fd, 59, err.Error())
		return
	}

	scgi_headers, query_string, err := unpack_request_bytes(request_bytes)

	if err != nil {
		write_response(fd, 59, err.Error())
		return
	}

	path := scgi_headers["PATH_INFO"]

	var pathcomps []string

	for _, s := range strings.Split(path, "/")[1:] {
		if len(s) > 0 {
			pathcomps = append(pathcomps, s)
		}
	}

	if len(pathcomps) == 0 || pathcomps[0] == "help" {
		handle_help(fd)
		return
	} else if pathcomps[0] == "threads" {
		handle_threads(fd, db, pathcomps, query_string)
		return
	} else if pathcomps[0] == "messages" {
		handle_messages(fd, db, pathcomps, query_string)
		return
	} else if pathcomps[0] == "search" {
		handle_search(fd, db, pathcomps, query_string)
		return
	} else {
		write_response(fd, 51, "not found")
		return
	}

}
