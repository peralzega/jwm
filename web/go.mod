// Berkas ini sengaja ada: menjadikan web/ modul terpisah agar `go ... ./...`
// di akar repositori tidak ikut memindai berkas .go di web/node_modules.
// Folder web/ tidak berisi kode Go.
module jwm/web

go 1.27
