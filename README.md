# GemThread

GemThread is an SCGI service intended for use with the Molly Brown Gemini server.

GemThread is designed to allow conversations to be held in Gemini-space.

## How Does It Work?

The primary use pattern is as follows:

1. A person (the "author") posts a page to their own Gemini server.
2. The author accesses the `/new` endpoint of a GemThread service (which does not have to be on their own Gemini server) and enters the `gemini://` URL pointing to their posted page.
4. The GemThread service fetches the page from the author's Gemini server, extracts a title and summary for the page, and creates a new thread for the page and any responses that are added.
5. The author may then add the GemThread thread URL to the original page and perhaps a note to let readers know that they may respond using the GemThread service (if desired).
6. A different person (the "responder") posts a response to the original page on their own Gemini server.
7. The responder access the `/respond` endpoint of the GemThread service and copies the `gemini://` URL pointing to their response page.
8. The GemThread service fetches the response page from the responder's Gemini server, extracts a title and summary, and adds the response information to the thread for the original page.

See the `help.gmi` file for much more information.

## System Requirements and Building

For now, GemThread is designed to be run as an SCGI service under the Molly Brown Gemini server. Other configurations will probably work but have not been tested. I'm happy to take pull requests to make GemThread work as a CGI or an FCGI service or even as a standalone server.

The easiest way to install GemThread is to clone this repository, set your GOPATH to point to somewhere appropriate, and run `go build`. Not ideal, and I'll probably make some binary releases once it makes sense. You will need the sample `gemthread.cfg` and `help.gmi` files anyway, however, so cloning the repository is not an entirely bad thing. (Also, I can get away with this because Molly Brown requires you to use `go get` and have a GOPATH already set up. Since you have already done that, these instructions should be a piece of cake. As always, pull requests are welcome if you want to make the setup process easier for others.)

The `gemthread.cfg` file should be self explanatory. Please log an issue if anything is unclear. The only thing you MUST change in the `gemthread.cfg` file is the `server_url` entry. It should point to the path of the SCGI service itself. In other words, if you have configured Molly Brown to use the `/gemthread` endpoint for the service, and your server name is `host.example.com`, you should configure the `server_url` entry as `gemini://host.example.com/gemthread`.

## Running

Configure Molly Brown to point to the SCGI socket defined in `gemthread.cfg`, and restart Molly Brown. Then run the gemthread server by doing:

```
gemthread -c /path/to/gemthread.cfg
```

## Questions? Comments? Anecdotes?

* Log an issue (always welcome); or
* Send email to the [gemthread mailing list](https://lists.sr.ht/~raphm/gemthread): [mailto://~raphm/gemthread@lists.sr.ht](mailto://~raphm/gemthread@lists.sr.ht)

## Mirrors

This repository is hosted at:

* https://git.sr.ht/~raphm/gemthread

It is also mirrored at:

* https://github.com/raphm/gemthread

Issues and pull requests are accepted in both locations.
