# feedbackapp

A very simple feedback app in golang, for real-time anonymous feedback during a conference or so: 
* audience can submit text (rate limited by IP, sanitized)
* viewing feedback is password protected, auto-refreshes
* simple admin interface (create/delete tags, show links/QR codes)


### run

* In a folder, create a htpasswd file with admin user: `htpasswd -c feedbackapp-logins.htpasswd <username>`.
* Download the [executable](https://github.com/wolfgangasdf/feedbackapp/releases) and run it.
* Go to `https://localhost:8000/admin`, first create a new tag (alphanumeric).
* Deployment: run it behind a reverse SSL proxy!


### settings file 
`feedbackapp-settings.json`: port defaults to 8000:
```
{
"port" : 8000
}
```


# build

```
go get -u github.com/go-bindata/go-bindata/v3/... 
# one of:
go-bindata -fs -prefix "static/" static/...        # put static files into bindata.go
go-bindata -debug -fs -prefix "static/" static/... # development: use normal files via bindata.go
# one of:
go build
GOOS=linux GOARCH=amd64 go build -o feedbackapp-linux-amd64 # cross-compile, e.g. for linux
go build && ./feedbackapp
# or update bindate & run
go-bindata -debug -fs -prefix "static/" static/... && go build && ./feedbackapp
```

### endpoints

```
/t/tag # show submit page
/submit/tag # submit (rate limited 5 per minute)
/feedback.html?t=tag&p=password # view feedback (needs js). Saves password in cookie, reloads page without password in URL.
/feedback/getafter?t=tag&p=password&afterindex=123 # get feedback items

behind login:
/admin
/admin/add/tag
/admin/rm/tag
/qrx?link=...
```


### uses
see go.mod

# license
MIT

