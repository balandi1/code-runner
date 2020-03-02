package server

import (
	"archive/tar"
	"archive/zip"
	"coderunner/constants"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"sync"
)

// Parses the client request and uploads the file.
func upload(w http.ResponseWriter, r *http.Request) {
	fmt.Println("File Upload Endpoint Hit")

	w.Header().Add("Content-Type", "application/json")
	responseString := readFormData(r)

	if len(responseString) <= 0 {
		responseString += `"Upload Status":"Successfully Uploaded File(s)"`
	}

	// Write the response to be sent to client.
	w.Header().Add("Content-Type", "application/json")
	_, err := io.WriteString(w, `{`+responseString+`}`)
	if err != nil {
		log.Println(err)
	}
}

// Reads the compressed file and invokes function to decompress it
func readFormData(r *http.Request) string {
	fileHeader := make([]byte, 512)

	// Returns the first file for the given key 'file'.
	file, handler, err := r.FormFile(constants.FormFileKey)
	if err != nil {
		responseString := `"File Error":"Error in retrieving the file"`
		log.Println("Error Retrieving the File", err)
		return responseString
	}

	// Read the command line arguments (additional parameters passed).
	for index := 1; index <= len(r.Form); index++ {
		argName := fmt.Sprintf("%s%d", "arg", index)
		arg := r.FormValue(argName)
		fmt.Println(arg)
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Println(err)
			return
		}
	}()

	if _, err := file.Read(fileHeader); err != nil {
		log.Println(err)
	}

	fmt.Printf("File Size: %+v\n", handler.Size)
	fmt.Printf("MIME Header: %+v\n", http.DetectContentType(fileHeader))

	return decompressFile(file, fileHeader, handler)
}

// Reads all files from the uploaded compressed file
func decompressFile(file multipart.File, fileHeader []byte, handler *multipart.FileHeader) string {
	// Based on the type of file compression, read the file.
	if http.DetectContentType(fileHeader) == "application/octet-stream" {
		// For tar or tar.gz file.
		unTarred := tar.NewReader(file)
		for {
			header, err := unTarred.Next()
			if err == io.EOF {
				// End of tar file.
				break
			}
			if err != nil {
				responseString := `"UnTar Error":"Error in un-tarring uploaded file"`
				log.Println(err)
				return responseString
			}
			fmt.Println(header.Name)
		}
	} else {
		// For zip file.
		unZipped, err := zip.NewReader(file, handler.Size)
		if err != nil {
			responseString := `"Unzip Error":"Error in unzipping uploaded file"`
			log.Println(err)
			return responseString
		}
		for _, file := range unZipped.File {
			fmt.Println(file.Name)
		}
	}

	return ""
}

func listenAndServe(wg *sync.WaitGroup, port string) {
	defer wg.Done()

	log.Printf("** Service Started on Port " + port + " **")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
func StartServer(port string) {

	var wg sync.WaitGroup

	// Start service at given port.
	http.HandleFunc("/upload", upload)

	wg.Add(1)
	go listenAndServe(&wg, port)
	wg.Wait()
}