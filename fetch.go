package main

import (
	"context"
	"errors"
	"io"
	"net/url"

	"git.sr.ht/~adnano/go-gemini"
)

func do(req *gemini.Request, via []*gemini.Request) (*gemini.Response, error) {
	client := gemini.Client{}
	ctx := context.Background()
	resp, err := client.Do(ctx, req)
	if err != nil {
		return resp, err
	}

	switch resp.Status.Class() {

	case gemini.StatusInput:
		return resp, errors.New("remote host requires input -- not supported")

	case gemini.StatusRedirect:
		via = append(via, req)
		if len(via) > 5 {
			return resp, errors.New("too many redirects")
		}

		target, err := url.Parse(resp.Meta)
		if err != nil {
			return resp, err
		}
		target = req.URL.ResolveReference(target)
		redirect := *req
		redirect.URL = target
		return do(&redirect, via)
	}

	return resp, err
}

func retrieve(addr string) (string, error) {

	req, err := gemini.NewRequest(addr)

	resp, err := do(req, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Handle response
	if resp.Status.Class() == gemini.StatusSuccess {

		response_body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(response_body), nil
	}

	return "", errors.New("something went wrong")

}
