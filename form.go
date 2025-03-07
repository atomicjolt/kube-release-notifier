package main

import (
    "bytes"
    "fmt"
    "net/http"
    "net/url"
        "os"
        "time"
        "strconv"
)

func notifyForm(name string, namespace string, environment string, tag string, message string) {
        formURL := os.Getenv("FORM_URL")

        now := time.Now()
    year, month, day := now.Date()

    data := url.Values{}
    data.Set("entry.1139585288", name)
    data.Set("entry.1206784702", tag)
    data.Set("entry.1975726925", environment)
    data.Set("entry.1384872629", message)
    data.Set("entry.1866769559_year", strconv.Itoa(year))
    data.Set("entry.1866769559_month", strconv.Itoa(int(month)))
    data.Set("entry.1866769559_day", strconv.Itoa(day))
    req, err := http.NewRequest("POST", formURL, bytes.NewBufferString(data.Encode()))
    if err != nil {
        fmt.Println("Error creating request:", err)
        return
    }

    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Println("Error sending request:", err)
        return
    }
    defer resp.Body.Close()

    fmt.Println("Response Status:", resp.Status)
}
