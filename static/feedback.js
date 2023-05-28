
"use strict"

window.onload = function() {

    var lastindex = 0

    let params = (new URL(document.location)).searchParams
    let tag = params.get("t")
    let password = params.get("p")

    // if url contains password, save as cookie and reload
    if (password != null) {
        document.cookie = tag + "=" + password
        console.log("have set cookie, reload...")
        window.location.href = window.location.href.split('?')[0] + "?t=" + tag
    } else {
        console.log("cookie: " + document.cookie)
        var match = document.cookie.match(new RegExp('(^| )' + tag + '=([^;]+)'));
        if (match) password = match[2];
    }

    var feedback = document.getElementById("feedback")
    var status = document.getElementById("status")

    function fetchit() {
        fetch('./feedback/getafter?t=' + tag + '&p='+ password + '&i=' + lastindex)
        .then(res => { 
            if (res.ok) return res.json()
            throw new Error('Something went wrong')
        })
        .then(data => {
            status.textContent = "refreshing..."
            if (data.feedback) {
                data.feedback.forEach((f) => {
                    var tr = document.createElement('tr')
                    tr.innerHTML = '<td>' + f + '</td>'
                    feedback.appendChild(tr)
                })
                lastindex = data.newlastindex
                // visual bell
                let c = document.body.style.backgroundColor
                document.body.style.backgroundColor = "#ffb300"
                setTimeout(function() { document.body.style.backgroundColor = c }, 250)
            }
            setTimeout(fetchit, 1000);
        })
        .catch((error) => {
            console.log(error)
            status.textContent = "error refreshing... reload!"
        })
    }

    if (password === null) {
        feedback.innerHTML="wrong password"
    } else {
        fetchit()
    }
}