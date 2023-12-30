async function newPost() {
    console.log("newpost called")
    const note = document.getElementById("note").value;
    console.log("n", note)

    if (note === "" || note.trim() === "enter a note here") {
        console.error("cannot post empty!!")
        window.alert("cannot post an empty note!!")
        return
    }

    const data = new FormData()
    data.append("post", JSON.stringify({
        "Note": note,
    }))

    if (document.getElementById("getFile").files.length > 0) {
        const img = document.getElementById("getFile").files[0];
        console.log("i", img)
        data.append("img", img)
    }

    const request = {
        method: "POST",
        body: data,
    }
    console.log("request", JSON.stringify(request))
    const resp = await fetch("/upload", request)
    console.log("done")

    if (resp.status === 200) {
        window.location = "/feed.html"
    } else {
        alert("there was an issue uploading, try agian")
    }
}