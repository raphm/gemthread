package main

import (
	"net/url"
	"regexp"
	"strings"
)

func scan_user(line string) string {

	tilde_rx := regexp.MustCompile(`~([^/?]+)`)
	tilde_matches := tilde_rx.FindStringSubmatch(line)

	if len(tilde_matches) > 0 {
		author := strings.TrimSpace(tilde_matches[0])
		if len(author) > 0 {
			return author
		}
	}

	userpath_rx := regexp.MustCompile(`(?i)/user[s]?/([^/?]+)`)
	userpath_matches := userpath_rx.FindStringSubmatch(line)

	if len(userpath_matches) > 1 {
		author := strings.TrimSpace(userpath_matches[1])
		if len(author) > 0 {
			return author
		}
	}

	return ""
}

type line_type int

const (
	line_text line_type = iota
	line_blank
	line_h3
	line_h2
	line_h1
	line_pre
	line_link
	line_quote
	line_bullet
)

func scan_line_type(line string) line_type {
	if len(strings.TrimSpace(line)) == 0 {
		return line_blank
	}
	if strings.HasPrefix(line, "###") {
		return line_h3
	}
	if strings.HasPrefix(line, "##") {
		return line_h2
	}
	if strings.HasPrefix(line, "#") {
		return line_h1
	}
	if strings.HasPrefix(line, "```") {
		return line_pre
	}
	if strings.HasPrefix(line, "=>") {
		return line_link
	}
	if strings.HasPrefix(line, ">") {
		return line_quote
	}
	if strings.HasPrefix(line, "* ") {
		return line_bullet
	}
	return line_text
}

// Parses post to determine author, title, summary, and whether it is prohibited to add the post
// Returns the parsed message values, whether or not it is OKAY to use the post, and the error, if any
func parse_post(rawurl string, post_text string) (GemThreadMessage, bool, error) {

	prohibit_rx := regexp.MustCompile(`(?i)^gemthread[-_\.:]prohibit`)
	author_rx := regexp.MustCompile(`(?i)^gemthread[-_\.:]author:[\s]*([\S]+.+)`)
	title_rx := regexp.MustCompile(`(?i)^gemthread[-_\.:]title:[\s]*([\S]+.+)`)
	summary_rx := regexp.MustCompile(`(?i)^gemthread[-_\.:]summary:[\s]*([\S]+.+)`)

	var msg GemThreadMessage

	u, err := url.Parse(rawurl)
	if err != nil {
		return msg, false, err
	}

	is_allowed := true
	msg.url = rawurl
	msg.author = scan_user(rawurl)
	if len(msg.author) == 0 {
		msg.author = u.Hostname()
	}

	lines := strings.Split(post_text, "\n")

	in_pre_block := false
	first_text_found := false

	for _, line := range lines {

		lt := scan_line_type(line)

		if lt == line_pre {
			in_pre_block = !in_pre_block
			continue
		}

		if in_pre_block {
			continue
		}

		if lt == line_text {

			prohibit_matches := prohibit_rx.FindStringSubmatch(line)
			if len(prohibit_matches) > 0 {
				is_allowed = false
				msg.author = ""
				msg.summary = ""
				msg.title = ""
				return msg, is_allowed, nil
			}

			author_matches := author_rx.FindStringSubmatch(line)
			if len(author_matches) > 0 {
				author := strings.TrimSpace(author_matches[1])
				if len(author) > 0 {
					msg.author = author
				}
				continue
			}

			summary_matches := summary_rx.FindStringSubmatch(line)
			if len(summary_matches) > 0 {
				summary := strings.TrimSpace(summary_matches[1])
				if len(summary) > 0 {
					msg.summary = summary
				}
				continue
			}

			title_matches := title_rx.FindStringSubmatch(line)
			if len(title_matches) > 0 {
				title := strings.TrimSpace(title_matches[1])
				if len(title) > 0 {
					msg.title = title
				}
				continue
			}

			if !first_text_found {
				msg.summary = strings.TrimSpace(line)
				first_text_found = true
			}

		} else if lt == line_h1 || lt == line_h2 || lt == line_h3 {
			if len(msg.title) == 0 {
				h_idx := 0
				for i, c := range line {
					h_idx = i
					if c != rune('#') {
						break
					}
				}
				line := line[h_idx:]
				line = strings.TrimSpace(line)
				msg.title = line
			}
		}
	}

	if len(msg.title) == 0 {
		msg.title = "Untitled"
	}

	return msg, is_allowed, nil
}
