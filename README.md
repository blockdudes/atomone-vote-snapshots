# Cosmos Prop 848 Snapshot (in Pure Golang!)

## Getting Started

### Prerequisites

- Ensure you have Go installed on your system.
- You will need the `cosmoshub-4-export-18010657.json` file.

### Setup

1. **Generate the JSON File**: 
   To create `cosmoshub-4-export-18010657.json`, run the following command:
   ```bash
   gaiad export --height 18010657 > cosmoshub-4-export-18010657.json 2>&1
   ```
   Place this file in the `data` directory of the project.

2. **Run the Application**: 
   To install the go modules run the following command: 
   ```bash
   go mod tidy
   ```

   Execute the following command to start the application:
   ```bash
   go run main.go
   ```
   On the first run, the application will set up the database and start the api server. On subsequent runs, it will directly start the server.

  #### Refresh DB

   If for any reason you want to refresh the database you can run the following command:
  
  ```bash
   go run main.go refresh
   ```


### Using the API

- **Accessing Vote Data**: 
  Once the server is running, it will listen on port 8080. You can access the vote information for a specific address by navigating to:
  ```
  http://localhost:8080/<your-address>
  ```
  You will receive a JSON response with the vote data, structured as follows:
  ```json
  {
    "shares": 101.0303060375804, // his total shares/voting power
    // how his validators voted
    "validators": [
      {
        "shares": 101.0303060375804,
        "validator_address": "cosmosvaloper1...",
        "vote_abstain": 0,
        "vote_no": 0,
        "vote_no_with_veto": 0,
        "vote_yes": 101.0303060375804
      }
    ],
    "vote_abstain": 0,
    "vote_no": 0,
    "vote_no_with_veto": 0,
    "vote_yes": 101.0303060375804,
    "voter": "cosmos10mkyedc...", // voter address
    "voted_by": 0 // 0 = user, 1 = validator
  }
  ```
