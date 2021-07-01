package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nParts = 8

// This channel provide number of bytes has been downloaded
var bytesDownloaded = make(chan int64)

// This type was created to counting number of bytes has been downloaded
// and put into bytesDownloaded channel
type writeCounter struct{}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	bytesDownloaded <- int64(n)
	return n, nil
}

//--------------------------------------------------------------------

func main() {
	var url string
	fmt.Printf("Enter URL: ")
	fmt.Scanf("%s", &url)

	start := time.Now()
	fmt.Printf("Validating URL...\n")
	legit, resp := validate(url)
	if legit {
		downloadFile(url, resp, nParts)
	}
	fmt.Printf("%s\n", time.Now().Sub(start))
	for {
		var c string
		fmt.Printf("\rType \"exit\" to quit program: ")
		fmt.Scanf("%s", &c)
		if c == "exit" {
			break
		}
	}
}

func validate(url string) (bool, *http.Response) {
	res, err := http.Head(url)

	if err != nil {
		fmt.Printf("Error occured while validating URL: %v\n", err)
		return false, res
	} else if res.StatusCode != http.StatusOK {
		fmt.Printf("Bad status: %d\n", res.StatusCode)
		return false, res
	} else {
		return true, res
	}
}

func downloadFile(url string, headResp *http.Response, nParts int) {
	// Get fileName
	fileName := "DownloadedFile"
	cd := headResp.Header.Get("Content-Disposition")
	if strings.Contains(cd, "filename") {
		fileName = cd[strings.Index(cd, "filename")+len("filename")+1:]
	} else {
		fileName = url[strings.LastIndex(url, "/")+1:]
	}

	if len(fileName) > 0 && fileName[0] == '"' {
		fileName = fileName[1:]
	}
	if len(fileName) > 0 && fileName[len(fileName)-1] == '"' {
		fileName = fileName[:len(fileName)-1]
	}

	// Counting length for each part
	fileSize, _ := strconv.Atoi(headResp.Header.Get("Content-Length"))
	var lenPart = make([]int, nParts)
	for i := 0; i < nParts-1; i++ {
		lenPart[i] = fileSize / nParts
		lenPart[nParts-1] = fileSize - lenPart[0]*(nParts-1)
	}

	// Download progress
	fmt.Printf("Start download: %s\n", fileName)

	go printProgress(bytesDownloaded, int64(fileSize))

	var wg sync.WaitGroup
	for i := 0; i < nParts; i++ {
		wg.Add(1)
		go func(i int) {
			file, err := os.Create(fileName + "_" + strconv.Itoa(i))
			if err != nil {
				fmt.Printf("Error while creating part %d: %v\n", i, err)
				return
			}
			defer file.Close()

			client := &http.Client{}
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				fmt.Printf("Error while creating request: %v\n", err)
				return
			} else {
				unit := headResp.Header.Get("Accept-Range")
				if unit == "" {
					unit = "bytes"
				}

				var rangeStart int
				if i > 0 {
					rangeStart = lenPart[i-1] * i
				} else {
					rangeStart = 0
				}
				rangeEnd := rangeStart + lenPart[i] - 1
				req.Header.Add("Range", unit+"="+strconv.Itoa(rangeStart)+"-"+strconv.Itoa(rangeEnd))

				res, err := client.Do(req)
				if err != nil {
					fmt.Printf("Error while sending download request for part %d: %v\n", i, err)
					return
				}
				defer res.Body.Close()

				counter := &writeCounter{}
				src := io.TeeReader(res.Body, counter)

				io.Copy(file, src)
				wg.Done()
			}
		}(i)
	}
	wg.Wait()

	merge(fileName, nParts)
	fmt.Printf("Finish.\n")
}

func merge(fileName string, nParts int) {
	fmt.Printf("\r%s\r", strings.Repeat(" ", 56))
	fmt.Printf("Merging %d parts into one...\n", nParts)
	fmt.Printf("\r%s\r", strings.Repeat(" ", 56))

	file, _ := os.Create(fileName)
	defer file.Close()
	for i := 0; i < nParts; i++ {
		partName := fileName + "_" + strconv.Itoa(i)
		part, _ := os.Open(partName)
		io.Copy(file, part)
		part.Close()
		os.Remove(partName)
	}
}

func printProgress(bytesDownloaded <-chan int64, fileSize int64) {
	var total int64 = 0
	sizeInMb := float64(fileSize) / (1024 * 1024)
	go func() {
		old := total
		for total < fileSize {
			rateMiB := float64(total-old) / (1024 * 1024)
			old = total
			fmt.Printf("\rDownloading... %.2f MiB of %.2f MiB at %.2f MiB/s", float64(total)/(1024*1024), sizeInMb, rateMiB)
			time.Sleep(1 * time.Second)
		}
	}()
	for n := range bytesDownloaded {
		total += n
	}
	return
}
