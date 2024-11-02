package main

import (
	"context"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"net/http"
	"regexp"
	"bytes"

	"github.com/gin-gonic/gin"
)

type Language struct {
    Name  string
    Value string
}

var languages = []Language{
    {Name: "Go", Value: "go"},
    {Name: "Python", Value: "python"},
}

var defaultCodes = map[string]string{
    "go": `package main

import "fmt"

func main() {
    var input string
    fmt.Scanln(&input)
    fmt.Println("Hello from Go! You entered:", input)
}
`,
    "python": `input_data = input()
print("Hello from Python! You entered:", input_data)`,
}

var editorModes = map[string]string{
    "go":     "golang",
    "python": "python",
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("*.html")

	r.GET("/", homeHandler)
	r.GET("/:language", homeHandler)
	r.POST("/run", runHandler)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}

func homeHandler(c *gin.Context) {
	language := c.Param("language")
    if language == "" {
        language = "go" // default language
    }

    defaultCode, ok := defaultCodes[language]
    if !ok {
        c.String(http.StatusNotFound, "Language not supported")
        return
    }

    tmplData := map[string]interface{}{
        "Languages":       languages,
        "DefaultCode":     template.JS(defaultCode),
        "EditorMode":      editorModes[language],
        "CurrentLanguage": language,
    }

    c.HTML(http.StatusOK, "base.html", tmplData)
}

func runHandler(c *gin.Context) {
	code := c.PostForm("code")
	language := c.PostForm("language")
	inputData := c.PostForm("inputData")

	// Process inputData: split by semicolons and join with newlines
	inputs := strings.Split(inputData, ";")
	for i, input := range inputs {
		inputs[i] = strings.TrimSpace(input)
	}
	processedInput := strings.Join(inputs, "\n")

	// Save code to a temp file
	tmpDir, err := ioutil.TempDir("", "code")
	if err != nil {
		c.String(500, "Could not create temp directory.")
		return
	}
	defer os.RemoveAll(tmpDir)

	var codeFile string
	var cmd *exec.Cmd

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    switch language {
    case "go":
        codeFile = filepath.Join(tmpDir, "main.go")
        err = ioutil.WriteFile(codeFile, []byte(code), 0644)
        if err != nil {
            c.String(500, "Could not write to temp file.")
            return
        }

        // Run the code
        cmd = exec.CommandContext(ctx, "go", "run", codeFile)
        cmd.Dir = tmpDir

    case "python":
        codeFile = filepath.Join(tmpDir, "main.py")
        err = ioutil.WriteFile(codeFile, []byte(code), 0644)
        if err != nil {
            c.String(500, "Could not write to temp file.")
            return
        }

        // Run the code
        cmd = exec.CommandContext(ctx, "python", codeFile)
        cmd.Dir = tmpDir

    default:
        c.String(400, "Unsupported language.")
        return
    }

    // Set the input data for the command
	cmd.Stdin = strings.NewReader(processedInput)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
 
	err = cmd.Run()
 
	output := stdout.String()
	errorOutput := stderr.String()
 
	// Sanitize outputs to remove temp directory paths
	output = sanitizeOutput(output, tmpDir)
	errorOutput = sanitizeOutput(errorOutput, tmpDir)
 
	escapedOutput := template.HTMLEscapeString(output)
 
	if err != nil {
		// Include sanitized error output if any
		if errorOutput != "" {
			escapedOutput += "\nError Output:\n" + template.HTMLEscapeString(errorOutput)
		}
		// Provide a generic error message
		escapedOutput += "\nError: An error occurred during execution."
	}
 
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(200, escapedOutput)
}

// Helper function to sanitize output
func sanitizeOutput(output string, tmpDir string) string {
    // Replace the temporary directory path with a placeholder
    sanitized := strings.ReplaceAll(output, tmpDir, "<tempdir>")

    // Remove any absolute Windows paths (e.g., C:/Users/...)
    sanitized = regexp.MustCompile(`[A-Za-z]:[/\\][^:\n]*`).ReplaceAllString(sanitized, "<filepath>")

    // Remove any absolute Unix paths (e.g., /home/user/...)
    sanitized = regexp.MustCompile(`(/[^:\n\s]*)+`).ReplaceAllString(sanitized, "<filepath>")

    return sanitized
}