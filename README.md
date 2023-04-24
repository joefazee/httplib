# Go HTTP CLIENT - httplib

httplib is a package that provides a simple and flexible way to create and send HTTP requests in Go. This package includes features like setting headers, customizing the request body, configuring the timeout, adding middleware, rate limiting, and handling retries with backoff. If you are looking for Guzzle or Axios for Golang, you are in the right place.

## Features

1. Creating a new request
2. Adding headers
3. Adding a JSON body
4. Adding a custom body
5. Setting the timeout
6. Using a custom HTTP client
7. Middleware
8. Reading a JSON response
9. Rate limiting
10. Retries with backoff
11. Sending the request
12. Chaining requests
13. Sending requests asynchronously
14. File upload

## Usage

### Creating a new request

To create a new request, use the `NewRequestBuilder` function:

```go
rb := httplib.NewRequestBuilder("GET", "https://api.example.com/data")
```

### Adding a JSON body

To add a JSON body to the request, use the `WithJSONBody` method:

```go
data := map[string]interface{}{
	"key": "value",
}
rb, err := rb.WithJSONBody(data)
```

### Adding a custom body

To add a custom body to the request, use the `WithCustomBody` method:

```go
rb.WithCustomBody(strings.NewReader("<xml>data</xml>"))
```

### Setting the timeout

To set the timeout for the request, use the `WithTimeout` method:

```go
rb.WithTimeout(10 * time.Second)
```

### Using custom HTTP client

To use a custom HTTP client for the request, use the `WithClient` method:

```go
rb.WithClient(myCustomClient)
```

### Middleware

To add a middleware function that will be executed on the response before returning it to the caller, use the Use method:

```go
rb.Use(myMiddlewareFunction)
```

### Reading a JSON response

To read the response body and unmarshal it into a target object, use the `ReadJSONResponse` method:

```go
response, err := rb.Send()
if err == nil {
	err = rb.ReadJSONResponse(response, &myTargetObject)
}
```

### Rate limiting

To set a rate limiter for the request, use the WithRateLimiter method:

```go
rb.WithRateLimiter(10, 5) // 10 requests per second with a burst of 5
```

### Retries with backoff

To set the number of retries and the backoff function for the request, use the WithRetry method:

```go
rb.WithRetry(3, httplib.ExponentialBackoff)
```

### Sending the request

To send the request and get the response, use the Send method:

```go
response, err := rb.Send()
```

### Chaining requests

To send a request and then use the response to build another request, use the SendAndChain method:

```go
response, err := rb.SendAndChain(myChainingFunction)
```

### Sending requests asynchronously

To send a request asynchronously and get a channel that will contain the response, use the SendAsync method:

```go
resultChan := rb.SendAsync()
```

## Complete Examples

### Fetching data from a REST API:

```go
package main

import (
    "fmt"
    "time"
    "github.com/joefazee/httplib"
)

type ApiResponse struct {
    Data []string `json:"data"`
}

func main() {
    rb := httplib.NewRequestBuilder("GET", "https://api.example.com/data")
    rb.WithHeader("Authorization", "Bearer <your-token>")
    rb.WithTimeout(10 * time.Second)

    response, err := rb.Send()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    var apiResponse ApiResponse
    err = rb.ReadJSONResponse(response, &apiResponse)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Println("Data:", apiResponse.Data)
}
```

### Posting data to a REST API:

```go
package main

import (
    "fmt"
    "time"
    "github.com/joefazee/httplib"
)

type PostData struct {
    Title string `json:"title"`
    Body  string `json:"body"`
}

func main() {
    postData := &PostData{
        Title: "My Title",
        Body:  "This is the body of my post.",
    }

    rb := httplib.NewRequestBuilder("POST", "https://api.example.com/posts")
    rb.WithHeader("Authorization", "Bearer <your-token>")
    rb.WithTimeout(10 * time.Second)

    _, err := rb.WithJSONBody(postData)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    response, err := rb.Send()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Println("Status:", response.Status)
}
```

### Fetching data with rate limiting and retries:

```go
package main

import (
    "fmt"
    "time"
    "github.com/joefazee/httplib"
)

type ApiResponse struct {
    Data []string `json:"data"`
}

func main() {
    rb := httplib.NewRequestBuilder("GET", "https://api.example.com/data")
    rb.WithHeader("Authorization", "Bearer <your-token>")
    rb.WithTimeout(10 * time.Second)
    rb.WithRateLimiter(5, 2) // 5 requests per second with a burst of 2
    rb.WithRetry(3, httplib.ExponentialBackoff)

    response, err := rb.Send()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    var apiResponse ApiResponse
    err = rb.ReadJSONResponse(response, &apiResponse)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Println("Data:", apiResponse.Data)
}
```

### Chainining request

```go
package main

import (
	"fmt"
	"github.com/joefazee/httplib"
	"log"
	"net/http"
)

type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type Todo struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
	UserId    int    `json:"userId"`
}

func main() {
	// Create a new GET request to retrieve a user
	userRB := httplib.NewRequestBuilder("GET", "https://jsonplaceholder.typicode.com/users/1")

	// Send the request and chain another request to retrieve the user's first to-do item
	response, err := userRB.SendAndChain(func(res *http.Response) (*httplib.RequestBuilder, error) {
		// Read JSON data from the response into a User struct
		var user User
		err := userRB.ReadJSONResponse(res, &user)
		if err != nil {
			return nil, err
		}

		// Print the retrieved user information
		fmt.Printf("Retrieved User: %+v\n", user)

		// Create a new GET request to retrieve the user's first to-do item
		todoURL := fmt.Sprintf("https://jsonplaceholder.typicode.com/todos?userId=%d", user.ID)
		todoRB := httplib.NewRequestBuilder("GET", todoURL)

		return todoRB, nil
	})

	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// Read JSON data from the response into a Todo struct
	var todos []Todo
	err = httplib.NewRequestBuilder("", "").ReadJSONResponse(response, &todos)
	if err != nil {
		log.Fatal(err)
	}

	// Print the retrieved to-do item
	fmt.Printf("Retrieved To-do Item: %+v\n", todos[0])
}
```

### Fetching data with rate limiting and retries:

```go
package main

import (
    "fmt"
    "time"
    "github.com/joefazee/httplib"
)

type ApiResponse struct {
    Data []string `json:"data"`
}

func main() {
    rb := httplib.NewRequestBuilder("GET", "https://api.example.com/data")
    rb.WithHeader("Authorization", "Bearer <your-token>")
    rb.WithTimeout(10 * time.Second)
    rb.WithRateLimiter(5, 2) // 5 requests per second with a burst of 2
    rb.WithRetry(3, httplib.ExponentialBackoff)

    response, err := rb.Send()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    var apiResponse ApiResponse
    err = rb.ReadJSONResponse(response, &apiResponse)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Println("Data:", apiResponse.Data)
}

```

### Asynchronously fetching multiple API endpoints:

```go
package main

import (
    "fmt"
    "sync"
    "github.com/joefazee/httplib"
)

func fetchUrl(url string, wg *sync.WaitGroup) {
    defer wg.Done()

    rb := httplib.NewRequestBuilder("GET", url)
    resultChan := rb.SendAsync()

    result := <-resultChan
    if result.Error != nil {
        fmt.Println("Error:", result.Error)
        return
    }

    fmt.Println("Fetched", url, "Status:", result.Response.Status)
}
```

### Uploading file to a server

1. First, create a simple HTTP server to handle file uploads:

```go
package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func main() {
	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(32 << 20) // 32MB max memory
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer file.Close()

		fileContent, err := ioutil.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		fmt.Println("Received file content:", string(fileContent))
		w.WriteHeader(http.StatusOK)
	})

	fmt.Println("Starting server on :8080...")
	http.ListenAndServe(":8080", nil)
}
```

This code creates a simple HTTP server that listens on port 8080 and handles file uploads at the /upload endpoint.

2. Next, create a client application to upload a file to the server using the httplib package:

```go
package main

import (
	"fmt"
	"github.com/joefazee/httplib"
	"log"
)

func main() {
	filePath := "/path/to/your/file.txt"
	rb := httplib.NewRequestBuilder("POST", "http://localhost:8080/upload")
	rb, err := rb.WithFile("file", filePath)
	if err != nil {
		log.Fatal(err)
	}

	response, err := rb.Send()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("File uploaded:", response.StatusCode)
}
```

### Upload a file to AWS S3 using the `httplib`

Install the AWS SDK for Go using:

```
go get -u github.com/aws/aws-sdk-go
```

Use the following code to generate a pre-signed URL:

```go
package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joefazee/httplib"
	"log"
	"time"
)

func main() {
	// Initialize a new session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a new S3 service client
	s3Client := s3.New(sess)

	// Generate the pre-signed URL
	req, _ := s3Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String("your-bucket-name"),
		Key:    aws.String("your-object-key"),
	})
	presignedURL, err := req.Presign(15 * time.Minute)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Pre-signed URL:", presignedURL)

	// Upload the file using the httplib package
	filePath := "/path/to/your/file.txt"
	rb := httplib.NewRequestBuilder("PUT", presignedURL)
	rb, err = rb.WithFile("file", filePath)
	if err != nil {
		log.Fatal(err)
	}

	response, err := rb.Send()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("File uploaded to S3:", response.StatusCode)
}
```
