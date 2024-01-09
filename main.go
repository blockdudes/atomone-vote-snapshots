package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/briandowns/spinner"
	sdk "github.com/cosmos/cosmos-sdk/types"
	badger "github.com/dgraph-io/badger/v4"
	json "github.com/goccy/go-json"

	"net/http"

	"github.com/logrusorgru/aurora/v4"

	"github.com/gin-gonic/gin"
)

type AccountVote struct {
	Voter      string  `json:"voter"`
	Yes        float64 `json:"vote_yes"`
	No         float64 `json:"vote_no"`
	NoWithVeto float64 `json:"vote_no_with_veto"`
	Abstain    float64 `json:"vote_abstain"`
}

type ValidatorVote struct {
	Shares           float64 `json:"shares"`
	ValidatorAddress string  `json:"validator_address"`
	VoteAbstain      float64 `json:"vote_abstain"`
	VoteNo           float64 `json:"vote_no"`
	VoteNoWithVeto   float64 `json:"vote_no_with_veto"`
	VoteYes          float64 `json:"vote_yes"`
}

type VotingDetails struct {
	Shares         float64         `json:"shares"`
	Validators     []ValidatorVote `json:"validators"`
	VoteAbstain    float64         `json:"vote_abstain"`
	VoteNo         float64         `json:"vote_no"`
	VoteNoWithVeto float64         `json:"vote_no_with_veto"`
	VoteYes        float64         `json:"vote_yes"`
	Voter          string          `json:"voter"`
	VotedBy        int             `json:"voted_by"` // 0 = user, 1 = validator
}

var DB *badger.DB

const DB_CHECK_KEY = "DB_CHECK"

func main() {

	refreshDB := len(os.Args) > 1 && os.Args[1] == "refresh"

	db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))
	if err != nil {
		log.Fatal(err)
	}
	DB = db
	defer db.Close()

	if refreshDB {
		clearDatabaseKey()
		fmt.Println("Starting DB Refresh!")
	}

	isDatabaseAlreadyStored := checkDatabaseKey()
	if !isDatabaseAlreadyStored || refreshDB {
		s := spinner.New(spinner.CharSets[43], 100*time.Millisecond)
		s.Start()
		fmt.Println(aurora.Blink(aurora.Cyan("Building the database, Please Wait few minutes!")))
		startTime := time.Now() // Capture start time
		calculateAndStoreData()
		duration := time.Since(startTime) // Calculate the duration
		s.Stop()
		fmt.Println(aurora.Sprintf(aurora.Bold(aurora.Cyan("Successfully stored the data in %.2f seconds!!!\n")), duration.Seconds()))
	}

	router := gin.Default()
	router.GET("/:address", fetchAddress)
	fmt.Println(aurora.Bold(aurora.Cyan("Web server started at localhost:8080 !!")))
	fmt.Println(aurora.Bold(aurora.Cyan("Example Address Data link -> localhost:8080/cosmos10mkyedcp04a7f4l603hqpuk3z57hg3xpepehaa").Hyperlink("http://localhost:8080/cosmos1n229vhepft6wnkt5tjpwmxdmcnfz55jv5c4tj7")))
	router.Run("localhost:8080")

}
func fetchAddress(c *gin.Context) {
	address := c.Param("address")
	fmt.Print(address)
	votingDetails := fetchData(address)
	c.IndentedJSON(http.StatusOK, votingDetails)
}
func fetchData(address string) VotingDetails {
	var retrievedDetails VotingDetails
	err := DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(address))
		if err != nil {
			return err
		}

		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(val, &retrievedDetails)
	})

	if err != nil {
		fmt.Print(err)
	}
	return retrievedDetails
}

func clearDatabaseKey() {
	DB.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(DB_CHECK_KEY))
	})
}

func checkDatabaseKey() bool {
	err := DB.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(DB_CHECK_KEY))
		if err != nil {
			return err
		}
		return nil
	})

	return err == nil
}

func calculateAndStoreData() {

	data, _ := os.ReadFile("data/cosmoshub-4-export-18010657.json")
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return
	}
	appState, found := getMap(jsonData, "app_state")
	if !found {
		return
	}

	staking, found := getMap(appState, "staking")
	if !found {
		return
	}
	// extractTopValidators(staking)
	topValidators, err := extractTopValidators(staking)
	if err != nil {
		// Printing the error
		fmt.Println("Error occurred extractTopValidators:", err)
		return
	}
	var topValidatorsAddressMap = make(map[string]string)
	for _, v := range topValidators {
		operatorAddress, found := v["operator_address"].(string)
		if !found {
			return
		}

		appAddress, found := v["app_address"].(string)
		if !found {
			return
		}

		topValidatorsAddressMap[operatorAddress] = appAddress
	}
	topDelegations, err := extractTopDelegations(staking, topValidatorsAddressMap)
	if err != nil {
		fmt.Println("Error occurred extractTopDelegations:", err)
		return
	}

	delegators, err := calculateDelegationGroupByAccount(topDelegations)
	if err != nil {
		fmt.Println("Error occurred calculateDelegationGroupByAccount:", err)
		return
	}

	votes, err := extractVotesProposal848(appState)
	if err != nil {
		fmt.Println("Error occurred extractVotesProposal848:", err)
		return
	}

	accountVotes, err := calculateAccountVote(delegators, votes)
	if err != nil {
		// Printing the error
		fmt.Println("Error occurred calculateAccountVote:", err)
		return
	}
	for key, details := range accountVotes {
		// Serialize VotingDetails
		serializedData, err := json.Marshal(details)
		if err != nil {
			log.Fatal(err)
		}

		// Store in BadgerDB
		err = DB.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), serializedData)
		})
		if err != nil {
			log.Fatal(err)
			return
		}
	}

	DB.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(DB_CHECK_KEY), []byte("STORED"))
	})
}

// // extractValidators navigates through the nested maps to extract the validators
func extractTopValidators(staking map[string]interface{}) ([]map[string]interface{}, error) {

	params, found := getMap(staking, "params")
	if !found {
		return nil, fmt.Errorf("'params' key not found")
	}
	max_validators_float, found := params["max_validators"].(float64)
	if !found {
		return nil, fmt.Errorf("'max_validators' key not found")
	}
	max_validators := int(max_validators_float)

	validators, found := getMapArray(staking, "validators")
	if !found {
		return nil, fmt.Errorf("'validators' key not found")
	}
	// Filter validators with status "BOND_STATUS_BONDED"
	var bondedValidators []map[string]interface{}
	for _, v := range validators {
		if v["status"] == "BOND_STATUS_BONDED" {
			bondedValidators = append(bondedValidators, v)
		}
	}
	// Sort validators by tokens in descending order
	sort.Slice(bondedValidators, func(i, j int) bool {
		tokensI, _ := strconv.Atoi(bondedValidators[i]["tokens"].(string))
		tokensJ, _ := strconv.Atoi(bondedValidators[j]["tokens"].(string))
		return tokensI > tokensJ
	})

	if len(bondedValidators) > max_validators {
		bondedValidators = bondedValidators[:max_validators]
	}
	for i, v := range bondedValidators {
		operatorAddress, found := v["operator_address"].(string)
		if !found {
			return nil, fmt.Errorf("'operatorAddress' key not found")
		}
		v["app_address"] = getAccountAddrFromVal(operatorAddress)
		bondedValidators[i] = v
	}

	return bondedValidators, nil
}

func extractTopDelegations(staking map[string]interface{}, topValidators map[string]string) ([]map[string]interface{}, error) {

	delegations, found := getMapArray(staking, "delegations")
	if !found {
		return nil, fmt.Errorf("'delegations' key not found")
	}
	var topDelegations []map[string]interface{}
	for _, d := range delegations {
		validatorAddress, found := d["validator_address"].(string)
		if !found {
			return nil, fmt.Errorf("'validator_address' key not found")
		}
		topValidatorAppAddress, ok := topValidators[validatorAddress]
		if ok {
			d["app_address"] = topValidatorAppAddress
			topDelegations = append(topDelegations, d)
		}
	}

	return topDelegations, nil
}

func extractVotesProposal848(appState map[string]interface{}) ([]map[string]interface{}, error) {
	gov, found := getMap(appState, "gov")
	if !found {
		return nil, fmt.Errorf("'gov' key not found")
	}

	votes, found := getMapArray(gov, "votes")
	if !found {
		return nil, fmt.Errorf("'votes' key not found")
	}
	votesProp848 := make([]map[string]interface{}, 0)
	for _, v := range votes {
		if v["proposal_id"] == "848" {
			votesProp848 = append(votesProp848, v)
		}
	}

	return votesProp848, nil
}

func calculateDelegationGroupByAccount(delegations []map[string]interface{}) (map[string]interface{}, error) {
	groupedDelegations := make(map[string]interface{})

	for _, delegation := range delegations {
		delegator := delegation["delegator_address"].(string)
		shares, err := strconv.ParseFloat(delegation["shares"].(string), 64)
		if err != nil {
			return nil, fmt.Errorf("'shares' key not found", err)
		}

		groupedDelegation, exists := groupedDelegations[delegator].(map[string]interface{})
		if !exists {
			groupedDelegation = make(map[string]interface{})
			// newGroupedDelegation := make(map[string]interface{})
			groupedDelegation["delegator_address"] = delegator
			groupedDelegation["shares"] = 0.0
			groupedDelegation["validators"] = make([]map[string]interface{}, 0)
			// groupedDelegation = newGroupedDelegation
		}

		currentShares, found := groupedDelegation["shares"].(float64)
		if !found {
			return nil, fmt.Errorf("'total shares' key not found")
		}

		totalShares := shares + currentShares
		groupedDelegation["shares"] = totalShares

		validators := groupedDelegation["validators"].([]map[string]interface{})
		validator := map[string]interface{}{
			"shares":            shares,
			"validator_address": delegation["validator_address"],
			"app_address":       delegation["app_address"],
			"validator_bond":    delegation["validator_bond"],
		}
		groupedDelegation["validators"] = append(validators, validator)
		groupedDelegations[delegator] = groupedDelegation
	}

	return groupedDelegations, nil
}
func calculateAccountVote(delegators map[string]interface{}, votes []map[string]interface{}) (map[string]VotingDetails, error) {
	accountVotes := make(map[string]AccountVote)

	for _, item := range votes {
		voter := item["voter"].(string)
		options := item["options"].([]interface{})

		voterInfo := AccountVote{
			Voter:      voter,
			Yes:        0.0,
			No:         0.0,
			NoWithVeto: 0.0,
			Abstain:    0.0,
		}
		for _, option := range options {
			opt := option.(map[string]interface{})
			votePercent, err := strconv.ParseFloat(opt["weight"].(string), 64)
			if err != nil {
				return nil, fmt.Errorf("error in vote calculation", err)
			}
			switch opt["option"] {
			case "VOTE_OPTION_YES":
				voterInfo.Yes = votePercent
			case "VOTE_OPTION_NO":
				voterInfo.No = votePercent
			case "VOTE_OPTION_NO_WITH_VETO":
				voterInfo.NoWithVeto = votePercent
			case "VOTE_OPTION_ABSTAIN":
				voterInfo.Abstain = votePercent
			}
		}
		accountVotes[voter] = voterInfo

	}
	accountFinalVotes := make(map[string]VotingDetails)

	for delegatorAddr, delegatorInterface := range delegators {
		delegator, ok := delegatorInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("delegator value error")
		}
		validators, _ := delegator["validators"].([]map[string]interface{})
		shares, _ := delegator["shares"].(float64)
		validatorsVote := make([]ValidatorVote, 0)
		voteYes, voteNo, voteNoWithVeto, voteAbstain := 0.0, 0.0, 0.0, 0.0
		votedBy := 1
		for _, validator := range validators {
			validatorAddress, _ := validator["validator_address"].(string)
			validatorAppAddress, _ := validator["app_address"].(string)
			validatorShares, _ := validator["shares"].(float64)

			validatorVote, ok := accountVotes[validatorAppAddress]
			if !ok {
				continue
			}
			validatorVoteYes := validatorVote.Yes * validatorShares
			validatorVoteNo := validatorVote.No * validatorShares
			validatorVoteNoWithVeto := validatorVote.NoWithVeto * validatorShares
			validatorVoteAbstain := validatorVote.Abstain * validatorShares
			validatorFullVote := ValidatorVote{
				ValidatorAddress: validatorAddress,
				Shares:           validatorShares,
				VoteYes:          validatorVoteYes,
				VoteNo:           validatorVoteNo,
				VoteNoWithVeto:   validatorVoteNoWithVeto,
				VoteAbstain:      validatorVoteAbstain,
			}

			voteYes += validatorVoteYes
			voteNo += validatorVoteNo
			voteNoWithVeto += validatorVoteNoWithVeto
			voteAbstain += validatorVoteAbstain
			validatorsVote = append(validatorsVote, validatorFullVote)
		}
		delegatorVote, ok := accountVotes[delegatorAddr]
		if ok {
			voteYes = delegatorVote.Yes * shares
			voteNo = delegatorVote.No * shares
			voteNoWithVeto = delegatorVote.NoWithVeto * shares
			voteAbstain = delegatorVote.Abstain * shares
			votedBy = 0
		}
		accountFinalVote := VotingDetails{
			Voter:          delegatorAddr,
			Shares:         shares,
			VoteYes:        voteYes,
			VoteNo:         voteNo,
			VoteNoWithVeto: voteNoWithVeto,
			VoteAbstain:    voteAbstain,
			Validators:     validatorsVote,
			VotedBy:        votedBy,
		}

		accountFinalVotes[delegatorAddr] = accountFinalVote

	}

	return accountFinalVotes, nil

}

func getMapArray(data map[string]interface{}, key string) ([]map[string]interface{}, bool) {
	valInterface, found := data[key].([]interface{})
	if !found {
		return nil, false
	}

	vals := make([]map[string]interface{}, len(valInterface))
	for i, v := range valInterface {
		val, ok := v.(map[string]interface{})
		if !ok {
			// Handle the case where the type assertion fails
			return nil, false
		}
		vals[i] = val
	}
	return vals, true

}

// getMap is a helper function to safely extract a map from an interface{}
func getMap(data map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, found := data[key]
	if !found {
		return nil, false
	}

	result, ok := val.(map[string]interface{})
	return result, ok
}

func getAccountAddrFromVal(validatorAddr string) string {
	valAddr, _ := sdk.ValAddressFromBech32(validatorAddr)
	accAddr, _ := sdk.AccAddressFromHexUnsafe(hex.EncodeToString(valAddr.Bytes()))
	return accAddr.String()
}
