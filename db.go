package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func db_drop_tables(db *sql.DB) error {

	sqlStmt := `
   drop table messages;
	drop table threads;
	`
	_, err := db.Exec(sqlStmt)
	return err
}

func db_create_tables(db *sql.DB) error {

	sqlStmt := `
	create table if not exists threads (
	   id integer not null primary key, 
		author text not null,
		title text not null,
		dt_created text not null,
		dt_updated text not null
	);
	create table if not exists messages (
	   id integer not null primary key, 
		url text not null,
		author text not null,
		title text not null,
		dt_created text not null,
		summary text
	);
	create unique index if not exists url_index on messages(url);
   create table if not exists responses (
                        threads_id integer,
                        messages_id integer,
		                  dt_created text not null,
                        foreign key(threads_id) references threads(id),
                        foreign key(messages_id) references messages(id)
                );
   create table if not exists originations (
                        messages_id integer,
                        threads_id integer,
                        foreign key(messages_id) references messages(id),
                        foreign key(threads_id) references threads(id)
                );
	`
	_, err := db.Exec(sqlStmt)
	return err

}

func db_open(database_path string, should_drop bool) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", database_path)
	if err != nil {
		return nil, err
	}

	if should_drop {
		db_drop_tables(db)
		if err != nil {
			return nil, err
		}
	}

	err = db_create_tables(db)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func db_find_message_by_id(db *sql.DB, msg_id int64) (GemThreadMessage, error) {

	var msg = GemThreadMessage{}

	stmt, err := db.Prepare("select * from messages where id = ?")
	if err != nil {
		return msg, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(msg_id)
	defer rows.Close()
	if err != nil {
		return msg, err
	}
	for rows.Next() {
		err = rows.Scan(&msg.id, &msg.url, &msg.author, &msg.title, &msg.dt_created, &msg.summary)
		if err != nil {
			return msg, err
		}
	}
	return msg, nil
}

func db_find_message_by_url(db *sql.DB, url string, partial_match bool) ([]GemThreadMessage, error) {

	var msgs = []GemThreadMessage{}

	stmt, err := db.Prepare("select * from messages where url like ?")
	if err != nil {
		return msgs, err
	}
	defer stmt.Close()

	arg := url

	if partial_match {
		arg = "%" + url + "%"
	}

	rows, err := stmt.Query(arg)
	if err != nil {
		return msgs, err
	}
	defer rows.Close()
	for rows.Next() {
		msg := GemThreadMessage{}
		err = rows.Scan(&msg.id, &msg.url, &msg.author, &msg.title, &msg.dt_created, &msg.summary)
		if err != nil {
			return msgs, err
		}
		msgs = append(msgs, msg)
	}
	err = rows.Err()
	if err != nil {
		return msgs, err
	}
	return msgs, nil
}

func db_list_messages(db *sql.DB, start int, count int, ascending bool) ([]GemThreadMessage, error) {

	var msgs = []GemThreadMessage{}

	order_by := "desc"

	if ascending {
		order_by = "asc"
	}

	stmt, err := db.Prepare(fmt.Sprintf("SELECT * FROM messages ORDER BY dt_created %s LIMIT ? OFFSET ?", order_by))
	if err != nil {
		return msgs, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(count, start)
	defer rows.Close()
	if err != nil {
		return msgs, err
	}
	for rows.Next() {
		msg := GemThreadMessage{}
		err = rows.Scan(&msg.id, &msg.url, &msg.author, &msg.title, &msg.dt_created, &msg.summary)
		if err != nil {
			return msgs, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func db_find_existing_message_by_url(db *sql.DB, tgt_url string) (GemThreadMessage, error) {

	var err error
	var msg = GemThreadMessage{}

	// Look for a message with this URL that already exists
	existing, err := db_find_message_by_url(db, tgt_url, false)

	if err != nil {
		return msg, err
	}

	if len(existing) > 1 {
		// Should never happen due to unique index
		err := errors.New(fmt.Sprintf("More than one message with the URL \"%s\" has been found. Unable to continue.", tgt_url))
		return msg, err
	} else if len(existing) == 1 {
		return existing[0], nil
	}

	return msg, nil
}

func db_insert_message(db *sql.DB, msg GemThreadMessage, tx *sql.Tx) (int64, error) {

	var err error
	var external_transaction bool

	if tx != nil {
		external_transaction = true
	} else {
		external_transaction = false
		tx, err = db.Begin()
		if err != nil {
			return -1, err
		}
	}

	msg_stmt, err := tx.Prepare("insert into messages(url, author, title, dt_created, summary) values(?, ?, ?, ?, ?)")
	if err != nil {
		return -1, err
	}

	defer msg_stmt.Close()

	msg_row, err := msg_stmt.Exec(msg.url, msg.author, msg.title, msg.dt_created, msg.summary)
	if err != nil {
		return -1, err
	}

	msg.id, err = msg_row.LastInsertId()
	if err != nil {
		return -1, err
	}

	if !external_transaction {
		err = tx.Commit()
	}

	return msg.id, err
}

func db_update_message(db *sql.DB, msg GemThreadMessage, tx *sql.Tx) (int64, error) {

	var err error
	var external_transaction bool

	if tx != nil {
		external_transaction = true
	} else {
		external_transaction = false
		tx, err = db.Begin()
		if err != nil {
			return -1, err
		}
	}

	msg_stmt, err := tx.Prepare("update messages set author = ?, title = ?, summary = ? where id = ?")
	if err != nil {
		return -1, err
	}

	defer msg_stmt.Close()

	_, err = msg_stmt.Exec(msg.author, msg.title, msg.summary, msg.id)
	if err != nil {
		return -1, err
	}

	thr, err := db_find_thread_by_originating_message_id(db, msg.id)
	if err != nil {
		return msg.id, err
	}
	if len(thr.dt_created) > 0 {
		// The message is an originating message for a thread. Update the thread author and title.
		thr_stmt, err := tx.Prepare("update threads set author = ?, title = ? where id = ?")
		if err != nil {
			return -1, err
		}

		defer thr_stmt.Close()

		_, err = thr_stmt.Exec(msg.author, msg.title, thr.id)
		if err != nil {
			return -1, err
		}
	}

	if !external_transaction {
		err = tx.Commit()
	}

	return msg.id, err
}

func db_delete_message(db *sql.DB, msg GemThreadMessage) (int64, error) {

	tx, err := db.Begin()
	if err != nil {
		return -1, err
	}

	msg_stmt, err := tx.Prepare("delete from messages where id = ?")
	if err != nil {
		return -1, err
	}
	defer msg_stmt.Close()
	_, err = msg_stmt.Exec(msg.id)
	if err != nil {
		return -1, err
	}

	msg_resp_stmt, err := tx.Prepare("delete from responses where messages_id = ?")
	if err != nil {
		return -1, err
	}
	defer msg_resp_stmt.Close()
	_, err = msg_resp_stmt.Exec(msg.id)
	if err != nil {
		return -1, err
	}

	msg_orig_stmt, err := tx.Prepare("delete from originations where messages_id = ?")
	if err != nil {
		return -1, err
	}
	defer msg_orig_stmt.Close()
	_, err = msg_orig_stmt.Exec(msg.id)
	if err != nil {
		return -1, err
	}

	err = tx.Commit()

	return msg.id, err
}

func db_insert_originating_message(db *sql.DB, thread_id int64, msg GemThreadMessage, tx *sql.Tx) (int64, error) {

	var err error
	var is_existing bool = false

	if tx == nil {
		err = errors.New("internal server error: external transaction is required to insert originating message")
		return -1, err
	}

	// Look for a message with this URL that already exists
	existing, err := db_find_existing_message_by_url(db, msg.url)

	if err != nil {
		return -1, err
	}

	// Some sort of test is required because "existing" might be empty.
	// We could check for url being empty, or some other such thing, but
	// it seems reasonable to simply double-check that the urls match.
	if existing.url == msg.url {
		// We already have a message with this URL. Copy the existing message's ID and dt_created values,
		// but do not overwrite the rest of the (passed-in) message as it could have been updated during
		// the parse step.
		is_existing = true
		msg.id = existing.id
		msg.dt_created = existing.dt_created
	}

	if len(msg.dt_created) == 0 {
		loc, _ := time.LoadLocation("UTC")
		now := time.Now().In(loc)
		msg.dt_created = now.Format("2006-01-02 15:04:05Z")
	}

	if is_existing {
		_, err := db_update_message(db, msg, tx)
		if err != nil {
			return -1, err
		}
	} else {
		msg.id, err = db_insert_message(db, msg, tx)
		if err != nil {
			return -1, err
		}
	}

	thr_orig_stmt, err := tx.Prepare("insert into originations(messages_id, threads_id) values(?, ?)")
	if err != nil {
		return -1, err
	}
	defer thr_orig_stmt.Close()
	_, err = thr_orig_stmt.Exec(msg.id, thread_id)
	if err != nil {
		return -1, err
	}

	return msg.id, err
}

func db_insert_response_message(db *sql.DB, thread_id int64, msg GemThreadMessage) (int64, error) {

	var err error

	tx, err := db.Begin()
	if err != nil {
		return -1, err
	}

	// Look for a message with this URL that already exists
	existing, err := db_find_existing_message_by_url(db, msg.url)

	if err != nil {
		return -1, err
	}

	// Some sort of test is required because "existing" might be empty.
	// We could check for url being empty, or some other such thing, but
	// it seems reasonable to simply double-check that the urls match.
	if existing.url == msg.url {
		// We already have a message with this URL. Copy the existing message's ID and dt_created values,
		// but do not overwrite the rest of the (passed-in) message as it could have been updated during
		// the parse step.
		msg.id = existing.id
		msg.dt_created = existing.dt_created
	}

	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	dt_created := now.Format("2006-01-02 15:04:05Z")
	if len(msg.dt_created) == 0 {
		msg.dt_created = dt_created
	}

	if existing.url == msg.url {
		_, err := db_update_message(db, msg, tx)
		if err != nil {
			return -1, err
		}
	} else {
		msg.id, err = db_insert_message(db, msg, tx)
		if err != nil {
			return -1, err
		}
	}

	thr_resp_stmt, err := tx.Prepare("insert into responses(threads_id, messages_id, dt_created) values(?, ?, ?)")
	if err != nil {
		return -1, err
	}
	defer thr_resp_stmt.Close()
	_, err = thr_resp_stmt.Exec(thread_id, msg.id, dt_created)
	if err != nil {
		return -1, err
	}

	thr_upd_stmt, err := tx.Prepare("update threads set dt_updated = ? where id = ?")
	if err != nil {
		return -1, err
	}
	defer thr_upd_stmt.Close()
	_, err = thr_upd_stmt.Exec(dt_created, thread_id)
	if err != nil {
		return -1, err
	}

	err = tx.Commit()

	return msg.id, err
}

func db_create_new_thread(db *sql.DB, msg GemThreadMessage) (int64, error) {

	var thr_id int64 = -1

	tx, err := db.Begin()
	if err != nil {
		return thr_id, err
	}

	existing_msg, err := db_find_existing_message_by_url(db, msg.url)
	if err != nil {
		return thr_id, err
	}

	if existing_msg.url == msg.url {
		thr, err := db_find_thread_by_originating_message_id(db, existing_msg.id)
		if err != nil {
			return thr_id, err
		}
		if len(thr.dt_created) > 0 {
			return thr.id, errors.New(fmt.Sprintf("Thread for this message already exists with ID %d", thr.id))
		}
		// We already have a message with this URL. Update the new (parsed) message
		// struct with the existing messages's ID and dt_created.
		msg.id = existing_msg.id
		msg.dt_created = existing_msg.dt_created
	}

	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	dt_created := now.Format("2006-01-02 15:04:05Z")
	if len(msg.dt_created) == 0 {
		msg.dt_created = dt_created
	}

	thr_stmt, err := tx.Prepare("insert into threads(author, title, dt_created, dt_updated) values(?, ?, ?, ?)")
	if err != nil {
		return thr_id, err
	}
	defer thr_stmt.Close()
	thr_row, err := thr_stmt.Exec(msg.author, msg.title, dt_created, "")
	if err != nil {
		return thr_id, err
	}
	thr_id, err = thr_row.LastInsertId()
	if err != nil {
		return -1, err
	}

	_, err = db_insert_originating_message(db, thr_id, msg, tx)
	if err != nil {
		return -1, err
	}

	err = tx.Commit()

	return thr_id, err

}

func db_find_thread_by_id(db *sql.DB, thr_id int64) (GemThreadThread, error) {

	var thr = GemThreadThread{}

	stmt, err := db.Prepare("select * from threads where id = ?")
	if err != nil {
		return thr, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(thr_id)
	defer rows.Close()
	if err != nil {
		return thr, err
	}
	for rows.Next() {
		err = rows.Scan(&thr.id, &thr.author, &thr.title, &thr.dt_created, &thr.dt_updated)
		if err != nil {
			return thr, err
		}
	}
	return thr, nil
}

func db_find_thread_by_originating_message_id(db *sql.DB, msg_id int64) (GemThreadThread, error) {

	var thr = GemThreadThread{}

	stmt, err := db.Prepare("select id, author, title, dt_created, dt_updated from threads INNER JOIN originations on threads.id = originations.threads_id WHERE originations.messages_id = ?;")
	if err != nil {
		return thr, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(msg_id)
	defer rows.Close()
	if err != nil {
		return thr, err
	}
	for rows.Next() {
		err = rows.Scan(&thr.id, &thr.author, &thr.title, &thr.dt_created, &thr.dt_updated)
		if err != nil {
			return thr, err
		}
	}
	return thr, nil
}

func db_find_threads_by_responding_message_id(db *sql.DB, msg_id int64) ([]GemThreadThread, error) {

	var thrs = []GemThreadThread{}

	stmt, err := db.Prepare("select id, author, title, threads.dt_created, dt_updated from threads INNER JOIN responses on threads.id = responses.threads_id WHERE responses.messages_id = ?;")
	if err != nil {
		return thrs, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(msg_id)
	defer rows.Close()
	if err != nil {
		return thrs, err
	}
	for rows.Next() {
		var thr = GemThreadThread{}
		err = rows.Scan(&thr.id, &thr.author, &thr.title, &thr.dt_created, &thr.dt_updated)
		if err != nil {
			return thrs, err
		}
		thrs = append(thrs, thr)
	}
	return thrs, nil
}

func db_list_threads(db *sql.DB, start int, count int, ascending bool, by_date_created bool) ([]GemThreadThread, error) {

	var thrs = []GemThreadThread{}

	order_by := "dt_updated"
	if by_date_created {
		order_by = "dt_created"
	}

	direction := "desc"
	if ascending {
		direction = "asc"
	}

	stmt, err := db.Prepare(fmt.Sprintf("SELECT * FROM threads ORDER BY %s %s LIMIT ? OFFSET ?", order_by, direction))
	if err != nil {
		return thrs, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(count, start)
	defer rows.Close()
	if err != nil {
		return thrs, err
	}
	for rows.Next() {
		var thr = GemThreadThread{}
		err = rows.Scan(&thr.id, &thr.author, &thr.title, &thr.dt_created, &thr.dt_updated)
		if err != nil {
			return thrs, err
		}
		thrs = append(thrs, thr)
	}
	return thrs, nil
}

func db_find_messages_for_thread(db *sql.DB, thr_id int64, ascending bool) ([]GemThreadMessage, error) {

	var msgs = []GemThreadMessage{}
	var originating_msg = GemThreadMessage{}

	orig_stmt, err := db.Prepare(fmt.Sprintf("select id, url, author, title, dt_created, summary from messages INNER JOIN originations on messages.id = originations.messages_id WHERE originations.threads_id = ? ORDER BY dt_created asc;"))

	if err != nil {
		return msgs, err
	}
	defer orig_stmt.Close()

	rows, err := orig_stmt.Query(thr_id)
	defer rows.Close()
	if err != nil {
		return msgs, err
	}
	for rows.Next() {
		var msg = GemThreadMessage{}
		err = rows.Scan(&msg.id, &msg.url, &msg.author, &msg.title, &msg.dt_created, &msg.summary)
		if err != nil {
			return msgs, err
		}
		originating_msg = msg
	}

	direction := "desc"
	if ascending {
		direction = "asc"
	}

	stmt, err := db.Prepare(fmt.Sprintf("select id, url, author, title, responses.dt_created, summary from messages INNER JOIN responses on messages.id = responses.messages_id WHERE responses.threads_id = ? ORDER BY responses.dt_created %s;", direction))

	if err != nil {
		return msgs, err
	}
	defer stmt.Close()

	rows, err = stmt.Query(thr_id)
	defer rows.Close()
	if err != nil {
		return msgs, err
	}

	if direction == "asc" {
		msgs = append(msgs, originating_msg)
	}

	for rows.Next() {
		var msg = GemThreadMessage{}
		err = rows.Scan(&msg.id, &msg.url, &msg.author, &msg.title, &msg.dt_created, &msg.summary)
		if err != nil {
			return msgs, err
		}
		msgs = append(msgs, msg)
	}

	if direction == "desc" {
		msgs = append(msgs, originating_msg)
	}

	return msgs, nil
}
