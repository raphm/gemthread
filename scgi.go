package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
)

func parse_query_string_to_map(query_string string) (map[string]string, error) {

	query_headers := make(map[string]string)

	query_map, err := url.ParseQuery(query_string)

	if err != nil {
		errstr := fmt.Sprintf("%s while parsing QUERY_STRING: '%s')", err.Error(), query_string)
		return nil, errors.New(errstr)
	}

	for k := range query_map {
		v := query_map.Get(k)
		s, err := url.QueryUnescape(v)
		if err != nil {
			errstr := fmt.Sprintf("%s while decoding QUERY_STRING value '%s' (query key: '%s')", err.Error(), v, k)
			return nil, errors.New(errstr)
		}
		query_headers[k] = s
	}

	return query_headers, nil
}

func unpack_request_bytes(raw_headers []byte) (map[string]string, string, error) {

	scgi_headers := make(map[string]string)
	split_headers := bytes.Split(raw_headers, []byte{0})

	for i := 0; i < len(split_headers)-1; i += 2 {
		scgi_headers[string(split_headers[i])] = string(split_headers[i+1])
	}

	query_str := scgi_headers["QUERY_STRING"]

	return scgi_headers, query_str, nil

}

func read_request_bytes(fd io.ReadWriteCloser) ([]byte, error) {

	reader := bufio.NewReader(fd)

	line, err := reader.ReadString(':')

	if err != nil {
		return nil, errors.New("error reading request length")
	}

	line_length := line[0 : len(line)-1]
	length, err := strconv.Atoi(line_length)

	if err != nil {
		return nil, errors.New("error parsing line length '" + line_length + "': " + err.Error())
	}

	raw_request := make([]byte, length)

	rlen, err := io.ReadFull(reader, raw_request)

	if err != nil {
		return nil, errors.New("error reading request: " + err.Error())
	}

	if rlen != length {
		return nil, errors.New("expected " + fmt.Sprint(length) + " bytes but read " + fmt.Sprint(rlen))
	}

	b, err := reader.ReadByte()

	if err != nil {
		return nil, errors.New("error reading last byte of request: " + err.Error())
	}

	// read and discard the trailing comma
	if b != ',' {
		return nil, errors.New("error parsing SCGI body: missing trailing comma")
	}

	return raw_request, nil
}

// The optional message MAY provide additional information on the failure and if given, a clieht SHOULD display it to the user.
// Lagrange currently does not, but Amfora does.

func write_response(fd io.ReadWriteCloser,
	status int,
	response_text string) (n int, err error) {
	var buf bytes.Buffer
	input_message := "%d %s\r\n"
	success_message := "%d text/gemini\r\n%s\r\n"
	error_message := "%d %s\r\n"
	if status < 20 {
		n, err := fmt.Fprintf(&buf, input_message, status, response_text)
		if err != nil {
			fmt.Printf(err.Error())
			return n, err
		}
	} else if status < 30 {
		n, err := fmt.Fprintf(&buf, success_message, status, response_text)
		if err != nil {
			fmt.Printf(err.Error())
			return n, err
		}
	} else {
		n, err := fmt.Fprintf(&buf, error_message, status, response_text)
		if err != nil {
			fmt.Printf(err.Error())
			return n, err
		}
	}
	return fd.Write(buf.Bytes())
}
