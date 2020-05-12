package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"coderunner/constants"
	"coderunner/environment"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type assignmentTestingInformation struct {
	CommandToExecute string
	CommandToCompile string
	WorkDir          string
	RootDir          string
	Output           string
	CmdlineArgs      map[string]string
}

var assignTestingInfo assignmentTestingInformation

func getSupportedLanguage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	language := os.Getenv(environment.SupportedLanguage)

	response, err := json.Marshal(language)
	if err != nil {
		log.Println(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)

}

// upload parses the client request and uploads the file.
func upload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	output := readFormData(r)

	if len(output) <= 0 {
		output += `"Upload Status":"Successfully Uploaded File(s)"`
	}

	response, err := json.Marshal(output)
	if err != nil {
		log.Println(err)
	}

	// Write the response to be sent to client.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

// buildRun builds and runs the assignment uploaded.
func build(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")

	var outputString string
	var currDir string
	var err error

	// Navigate to the assignment working directory.
	outputString, currDir = navigateToWorkDir()
	if outputString == "" {
		// Execute the compile command.
		outputString, err = runCommand(assignTestingInfo.CommandToCompile)
		if err != nil {
			log.Println("error while building the assignment", err)
			outputString = err.Error()
		}

		// Navigate back to the code-runner working directory after successful execution.
		err = os.Chdir(currDir)
		if err != nil {
			log.Println("error while navigating to the current directory", err)
		}
	}

	if err == nil {
		outputString = "Compiled successfully"
	}
	response, err := json.Marshal(outputString)
	if err != nil {
		log.Println(err)
	}
	// Write the response to be sent to client.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

// buildRun builds and runs the assignment uploaded.
func run(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")

	var outputString string
	var currDir string
	var err error

	// Navigate to the assignment working directory.
	outputString, currDir = navigateToWorkDir()
	if outputString == "" {

		// Append the command line arguments to run command.
		runCmd := assignTestingInfo.CommandToExecute
		for _, value := range assignTestingInfo.CmdlineArgs {
			runCmd = fmt.Sprintf("%s %s", runCmd, value)
		}
		// Execute the assignment run command.
		outputString, err = runCommand(runCmd)
		if err != nil {
			log.Println("error while executing the assignment", err)
			outputString = err.Error()
		}

		// Navigate back to the code-runner working directory after successful execution.
		err = os.Chdir(currDir)
		if err != nil {
			log.Println("error while navigating to the current directory", err)
		}
	}

	response, err := json.Marshal(outputString)
	if err != nil {
		log.Println(err)
	}
	// Write the response to be sent to client.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(response)
}

// navigateToWorkDir navigates to the provided working directory of the assignment.
func navigateToWorkDir() (string, string) {
	workDir := filepath.Join(assignTestingInfo.RootDir, assignTestingInfo.WorkDir)
	currDir, err := os.Getwd()
	if err != nil {
		log.Println("error while getting current directory", err)
		return fmt.Sprintf("Error while navigating to working directory"), ""
	}
	err = os.Chdir(filepath.Join(currDir, constants.AssignmentsDir, workDir))
	if err != nil {
		log.Println("error while navigating to the working directory: ", err)
		return fmt.Sprintf("Error while navigating to working directory"), ""
	}
	return "", currDir
}

// runCommand runs the provided command.
func runCommand(cmdStr string) (string, error) {
	var out bytes.Buffer
	var stderr bytes.Buffer
	var output string
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		output = fmt.Sprintf("%v\n%v", out.String(), stderr.String())
		return output, err
	}

	output = fmt.Sprintf("%v", out.String())
	return output, nil
}

// readFormData reads the compressed assignment submission and extracts the contents.
func readFormData(r *http.Request) string {
	fileHeader := make([]byte, 512)

	// Get the first file for the given key 'file'.
	file, handler, err := r.FormFile(constants.FormFileKey)
	if err != nil {
		response := `"File Error":"Error in retrieving the file"`
		log.Println("error retrieving the file", err)
		return response
	}

	// Get the command line arguments.
	assignTestingInfo.CmdlineArgs = make(map[string]string)
	for index := 1; index <= len(r.Form); index++ {
		keyName := fmt.Sprintf("%s%d", constants.CmdArgKeyName, index)
		key := r.FormValue(keyName)

		argName := fmt.Sprintf("%s%d", constants.CmdArgValueName, index)
		arg := r.FormValue(argName)
		assignTestingInfo.CmdlineArgs[key] = arg
	}

	// Read the working directory, command to compile and command to run.
	assignTestingInfo.CommandToCompile = r.FormValue(constants.CompileCmdKey)
	assignTestingInfo.CommandToExecute = r.FormValue(constants.RunCmdKey)
	assignTestingInfo.WorkDir = r.FormValue(constants.WorkDirKey)

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

	// Decompress the file and return its response.
	return decompressFile(file, fileHeader, handler)
}

// decompressFile reads and stores all files from the uploaded compressed file.
func decompressFile(file multipart.File, fileHeader []byte, handler *multipart.FileHeader) string {

	// Read the file based on the type of file compression.
	assignTestingInfo.RootDir = strings.TrimSuffix(handler.Filename, path.Ext(handler.Filename))

	if http.DetectContentType(fileHeader) == constants.ZipMimeFileType {
		// Read zip file.
		unZipped, err := zip.NewReader(file, handler.Size)
		if err != nil {

			responseString := `"Unzip Error":"Error in unzipping uploaded file"`
			log.Println("error in unzipping file", err)
			return responseString
		}
		return storeUnzippedFiles(unZipped)

	} else if http.DetectContentType(fileHeader) == constants.TarGzMimeFileType {
		// Read tar.gz file.
		assignTestingInfo.RootDir = strings.TrimSuffix(assignTestingInfo.RootDir,
			path.Ext(assignTestingInfo.RootDir))
		fileReader, err := handler.Open()
		gZipReader, err := gzip.NewReader(fileReader)
		if err != nil {
			responseString := `"Untar Error":"Error in untaring uploaded file"`
			log.Println("error in untaring file", err)
			return responseString
		}
		unTarred := tar.NewReader(gZipReader)
		return storeUnTarredFiles(unTarred)

	} else {
		// Read tar file.
		var fileReader io.ReadCloser = file
		unTarred := tar.NewReader(fileReader)
		return storeUnTarredFiles(unTarred)
	}
}

// storeUnTarredFiles stores unTared files to 'assignments' directory.
func storeUnTarredFiles(unTarred *tar.Reader) string {

	errResponse := `"UnTar Error":"Error in un-tarring uploaded file"`
	dest := filepath.Join(constants.AssignmentsDir, assignTestingInfo.RootDir)
	for {
		header, err := unTarred.Next()
		if err == io.EOF {
			// End of tar file.
			break
		}
		if err != nil {
			log.Println("unTar error: ", err)
			return errResponse
		}

		filename := header.Name
		switch header.Typeflag {
		case tar.TypeDir:
			err := os.MkdirAll(filepath.Join(dest, filename), os.FileMode(header.Mode)) // or use 0755 if you prefer
			if err != nil {
				log.Println("unTar error: ", err)
				return errResponse
			}

		case tar.TypeReg:
			err := os.MkdirAll(filepath.Join(dest, filepath.Dir(filename)), os.FileMode(header.Mode))
			writer, err := os.Create(filepath.Join(dest, filename))
			if err != nil {
				log.Println("unTar error: ", err)
				return errResponse
			}

			_, err = io.Copy(writer, unTarred)
			if err != nil {
				log.Println("unTar error: ", err)
				return errResponse
			}

			err = os.Chmod(filepath.Join(dest, filename), os.FileMode(header.Mode))

			if err != nil {
				log.Println("unTar error: ", err)
				return errResponse
			}

			writer.Close()
		default:
			log.Println("unable to unTar type : ", header.Typeflag)
			return errResponse
		}
	}
	return ""
}

// storeUnzippedFiles stores unzipped files to 'assignments' directory.
func storeUnzippedFiles(unZipped *zip.Reader) string {
	dest := filepath.Join(constants.AssignmentsDir, assignTestingInfo.RootDir)

	errorResponse := `"Unzip Error":"Error in unzipping uploaded file"`

	for _, file := range unZipped.File {
		fPath := filepath.Join(dest, file.Name)

		if !strings.HasPrefix(fPath, filepath.Clean(dest)+string(os.PathSeparator)) {
			log.Println("unzip error: illegal filepath")
			return errorResponse
		}

		if file.FileInfo().IsDir() {
			err := os.MkdirAll(fPath, os.ModePerm)
			if err != nil {
				log.Println("unzip error: ", err)
				return errorResponse
			}
			continue
		}

		err := os.MkdirAll(filepath.Dir(fPath), os.ModePerm)
		if err != nil {
			log.Println("unzip error: ", err)
			return errorResponse
		}

		outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			log.Println("unzip error: ", err)
			return errorResponse
		}

		fileReader, err := file.Open()
		if err != nil {
			log.Println("unzip error: ", err)
			return errorResponse
		}

		_, err = io.Copy(outFile, fileReader)

		// Close the file without defer to close before next iteration of loop.
		outFile.Close()
		fileReader.Close()

		if err != nil {
			log.Println("unzip error: ", err)
			return errorResponse
		}

	}
	return ""
}

// listenAndServe listens to requests on the given port number.
func listenAndServe(wg *sync.WaitGroup, port string) {
	defer wg.Done()

	log.Printf("** Service Started on Port " + port + " **")
	http.Handle("/", http.FileServer(http.Dir("./client")))
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Println(err)
	}
}

// StartServer starts service at given port.
func StartServer(port string) {

	var wg sync.WaitGroup

	http.HandleFunc("/getSupportedLanguage", getSupportedLanguage)
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/build", build)
	http.HandleFunc("/run", run)
	wg.Add(1)
	go listenAndServe(&wg, port)
	wg.Wait()
}
