package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func fetchAttachments(inputArchive string, outputArchive string) error {

	// Check the parameters.
	if len(inputArchive) == 0 {
		fmt.Printf("fetch-attachments command requires --input-archive to be specified.\n")
		os.Exit(1)
	}
	if len(outputArchive) == 0 {
		fmt.Printf("fetch-attachments command requires --output-archive to be specified.\n")
		os.Exit(1)
	}

	// Open the input archive.
	r, err := zip.OpenReader(inputArchive)
	if err != nil {
		fmt.Printf("Could not open input archive for reading: %s\n", inputArchive)
		os.Exit(1)
	}
	defer r.Close()

	// Open the output archive.
	f, err := os.Create(outputArchive)
	if err != nil {
		fmt.Printf("Could not open the output archive for writing: %s\n\n%s", outputArchive, err)
		os.Exit(1)
	}
	defer f.Close()

	// Create a zip writer on the output archive.
	w := zip.NewWriter(f)

	// Run through all the files in the input archive.
	for _, file := range r.File {

		// Open the file from the input archive.
		inReader, err := file.Open()
		if err != nil {
			fmt.Printf("Failed to open file in input archive: %s\n\n%s", file.Name, err)
			os.Exit(1)
		}

		// Read the file into a byte array.
		inBuf, err := ioutil.ReadAll(inReader)
		if err != nil {
			fmt.Printf("Failed to read file in input archive: %s\n\n%s", file.Name, err)
		}

		// Now write this file to the output archive.
		outFile, err := w.Create(file.Name)
		if err != nil {
			fmt.Printf("Failed to create file in output archive: %s\n\n%s", file.Name, err)
			os.Exit(1)
		}
		_, err = outFile.Write(inBuf)
		if err != nil {
			fmt.Printf("Failed to write file in output archive: %s\n\n%s", file.Name, err)
		}

		// Check if the file name matches the pattern for files we need to parse.
		splits := strings.Split(file.Name, "/")
		if len(splits) == 2 && !strings.HasPrefix(splits[0], "__") && strings.HasSuffix(splits[1], ".json") {
			// Parse this file.
			err = processChannelFile(w, file, inBuf)
			if err != nil {
				fmt.Printf("%s", err)
				os.Exit(1)
			}
		}
	}

	// Close the output zip writer.
	err = w.Close()
	if err != nil {
		fmt.Printf("Failed to close the output archive.\n\n%s", err)
	}

	return nil
}

func processChannelFile(w *zip.Writer, file *zip.File, inBuf []byte) error {

	// Parse the JSON of the file.
	var posts []SlackPost
	if err := json.Unmarshal(inBuf, &posts); err != nil {
		return errors.New("Couldn't parse the JSON file: " + file.Name + "\n\n" + err.Error() + "\n")
	}

	// Loop through all the posts.
	for _, post := range posts {
		// We only care about file_share posts.
		if post.Subtype != "file_share" {
			continue
		}

		// Check there's a File property.
		if post.File == nil {
			log.Print("++++++ file_share post has no File property: " + post.Ts + "\n")
			continue
		}

		// Check there's an Id, Name and either UrlPrivateDownload or UrlPrivate property.
		if len(post.File.Id) < 1 || len(post.File.Name) < 1 || !(len(post.File.UrlPrivate) > 0 || len(post.File.UrlPrivateDownload) > 0) {
			log.Print("++++++ file_share post has missing properties on it's File object: " + post.Ts + "\n")
			continue
		}

		// Figure out the download URL to use.
		var downloadUrl string
		if len(post.File.UrlPrivateDownload) > 0 {
			downloadUrl = post.File.UrlPrivateDownload
		} else {
			downloadUrl = post.File.UrlPrivate
		}

		// Build the output file path.
		outputPath := "__uploads/" + post.File.Id + "/" + post.File.Name

		// Create the file in the zip output file.
		outFile, err := w.Create(outputPath)
		if err != nil {
			log.Print("++++++ Failed to create output file in output archive: " + outputPath + "\n\n" + err.Error() + "\n")
			continue
		}

		// Fetch the file.
		response, err := http.Get(downloadUrl)
		if err != nil {
			log.Print("++++++ Failed to donwload the file: " + downloadUrl)
			continue
		}
		defer response.Body.Close()

		// Save the file to the output zip file.
		_, err = io.Copy(outFile, response.Body)
		if err != nil {
			log.Print("++++++ Failed to write the downloaded file to the output archive: " + downloadUrl + "\n\n" + err.Error() + "\n")
		}

		// Success at last.
		fmt.Printf("Downloaded attachment into output archive: %s.\n", post.File.Id)
	}

	return nil
}
