package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "google.golang.org/api/sheets/v4"
    "google.golang.org/api/option"
    
    "golang.org/x/oauth2/google"
)

func notifySheet(name string, namespace string, environment string, tag string, message string) {
    spreadsheetID := os.Getenv("SPREADSHEET_ID")
    if spreadsheetID == "" {
        log.Println("SPREADSHEET_ID is not set")
        return
    }
    tokenStr := os.Getenv("GOOGLE_SHEETS_TOKEN")
    if tokenStr == "" {
        log.Println("GOOGLE_SHEETS_TOKEN environment variable is not set")
        return
    }

    cfg, err := google.JWTConfigFromJSON([]byte(tokenStr), sheets.SpreadsheetsScope)
    if err != nil {
        fmt.Errorf("unable to parse client secret file to config: %w", err)
        return
    }
    client := cfg.Client(context.Background())

    srv, err := sheets.NewService(context.Background(), option.WithHTTPClient(client))
    if err != nil {
        fmt.Errorf("unable to retrieve Sheets client: %w", err)
        return
    }
 
    // Get spreadsheet details to find the first sheet's name
    spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
    if err != nil {
        log.Printf("Unable to retrieve spreadsheet details: %v", err)
        return
    }

    if len(spreadsheet.Sheets) == 0 {
        log.Printf("No sheets found in the spreadsheet")
        return
    }

    // Get the first sheet's title
    firstSheetName := spreadsheet.Sheets[0].Properties.Title
    fmt.Printf("Appending to the first sheet: %s\n", firstSheetName)

    // Define range (appending to first available row)
    rangeToAppend := firstSheetName + "!A1"

    currentTime := time.Now().Format("2006-01-02 15:04:05")

    values := [][]interface{}{
        {currentTime, name, tag, environment, currentTime, message},
    }

    // Prepare request
    rb := &sheets.ValueRange{
        Values: values,
    }

    _, err = srv.Spreadsheets.Values.Append(spreadsheetID, rangeToAppend, rb).
        ValueInputOption("USER_ENTERED"). // Use "RAW" for no formatting
        InsertDataOption("INSERT_ROWS").
        Do()

    if err != nil {
        log.Printf("Unable to append data: %v", err)
        return
    }

    fmt.Println("Row appended successfully!")
}
