### teaspoon
---
This was my final project for [Collective Action School 2023](https://school.logicmag.io). To read more about it, read the about page [here](https://www.arushibandi.com/teaspoon/home.html).

To run this server, you must have [golang](https://go.dev/doc/install) installed and a [tailscale](https://tailscale.com) account. To run, `go run main.go` from `teaspoon/`. On the first run, you will be prompted to either re-run with an auth key, or log in to tailscale. After that, you can ignore the key and run with just `go run main.go`.

The server should show up as a machine named "teaspoon" in your tailscale admin console.

---

Images get written to `/img` and post content gets written to `/post`. Data used by tailscale is saved in `data/` and this folder contains sensitive information.