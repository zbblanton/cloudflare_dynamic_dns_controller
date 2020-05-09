package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
)

type Cloudflare struct {
	AuthEmail string
	AuthToken string
	ZoneID    string
	mux       sync.Mutex
}

type CloudflareRecordReq struct {
	RecordType string `json:"type"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	TTL        int    `json:"ttl"`
	Proxied    bool   `json:"proxied"`
}

type CloudflareRecord struct {
	ID         string `json:"id"`
	RecordType string `json:"type"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	TTL        int    `json:"ttl"`
	Proxied    bool   `json:"proxied"`
}

type CloudflareRespResultInfo struct {
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

type CloudflareRespError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type CloudflareResp struct {
	Success    bool                     `json:"success"`
	Result     []CloudflareRecord       `json:"result"`
	ResultInfo CloudflareRespResultInfo `json:"result_info"`
	Errors     []CloudflareRespError    `json:"errors"`
}

type CloudflareSingleResp struct {
	Success    bool                     `json:"success"`
	Result     CloudflareRecord         `json:"result"`
	ResultInfo CloudflareRespResultInfo `json:"result_info"`
	Errors     []CloudflareRespError    `json:"errors"`
}

func NewCloudflare(authEmail, authToken, zoneID string) Cloudflare {
	return Cloudflare{
		AuthEmail: authEmail,
		AuthToken: authToken,
		ZoneID:    zoneID,
	}
}

func (c *Cloudflare) CallAPI(method, path string, body io.Reader) error {
	// c.mux.Lock()
	// api_url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records/" + id
	// client := &http.Client{}
	// req, err := http.NewRequest("PATCH", api_url, body)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// req.Header.Add("X-Auth-Email", c.AuthEmail)
	// req.Header.Add("X-Auth-Key", c.AuthToken)
	// req.Header.Add("Content-type", "application/json")
	// c.mux.Unlock()

	// resp, err := client.Do(req)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer resp.Body.Close() //Close the resp body when finished

	// r := Cf_data{}
	// json.NewDecoder(resp.Body).Decode(&r)

	// //Check if success, print errors from api if not.
	// if !r.Success {
	// 	for _, e := range r.Errors {
	// 		log.Printf("Error code %d: %s\n", e.Code, e.Message)
	// 	}
	// 	return fmt.Errorf("Api call failed")
	// }

	// /*
	// 	Marked to be deleted after testing.
	// 	resp_data, err := ioutil.ReadAll(resp.Body)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// */

	return nil
}

//ListTXTRecords - List all TXT records
func (c *Cloudflare) ListTXTRecords() (records []CloudflareRecord, err error) {
	c.mux.Lock()
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records?type=TXT"
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []CloudflareRecord{}, err
	}
	req.Header.Add("X-Auth-Email", c.AuthEmail)
	req.Header.Add("X-Auth-Key", c.AuthToken)
	req.Header.Add("Content-type", "application/json")
	c.mux.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return []CloudflareRecord{}, err
	}
	defer resp.Body.Close() //Close the resp body when finished

	respBody := CloudflareResp{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return []CloudflareRecord{}, err
	}

	// //Check if success, print errors from api if not.
	// if !respBody.Success {
	// 	// for _, e := range respBody.Errors {
	// 	// 	log.Printf("Error code %d: %s\n", e.Code, e.Message)
	// 	// }
	// 	return fmt.Errorf("Api call failed")
	// }

	if len(respBody.Result) == 0 {
		return []CloudflareRecord{}, errors.New("Could not find any TXT records")
	}

	return respBody.Result, nil
}

//GetRecord - Get A record info
func (c *Cloudflare) GetRecord(recordType, recordName string) (record CloudflareRecord, err error) {
	c.mux.Lock()
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records?type=" + recordType + "&name=" + recordName
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return CloudflareRecord{}, err
	}
	req.Header.Add("X-Auth-Email", c.AuthEmail)
	req.Header.Add("X-Auth-Key", c.AuthToken)
	req.Header.Add("Content-type", "application/json")
	c.mux.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return CloudflareRecord{}, err
	}
	defer resp.Body.Close() //Close the resp body when finished

	respBody := CloudflareResp{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return CloudflareRecord{}, err
	}

	// //Check if success, print errors from api if not.
	// if !respBody.Success {
	// 	// for _, e := range respBody.Errors {
	// 	// 	log.Printf("Error code %d: %s\n", e.Code, e.Message)
	// 	// }
	// 	return fmt.Errorf("Api call failed")
	// }

	if len(respBody.Result) == 0 {
		return CloudflareRecord{}, errors.New("Could not find any records for " + recordName)
	}

	return respBody.Result[0], nil
}

//CreateARecord - Create an A record
func (c *Cloudflare) CreateARecord(name, content string, ttl int, proxied bool) (record CloudflareRecord, err error) {
	newRecord := CloudflareRecordReq{
		RecordType: "A",
		Name:       name,
		Content:    content,
		TTL:        ttl,
		Proxied:    proxied,
	}
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(newRecord)

	c.mux.Lock()
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records"
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return CloudflareRecord{}, err
	}
	req.Header.Add("X-Auth-Email", c.AuthEmail)
	req.Header.Add("X-Auth-Key", c.AuthToken)
	req.Header.Add("Content-type", "application/json")
	c.mux.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return CloudflareRecord{}, err
	}
	defer resp.Body.Close() //Close the resp body when finished

	respBody := CloudflareSingleResp{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return CloudflareRecord{}, err
	}

	//Check if success, print errors from api if not.
	var errMessage string
	if !respBody.Success {
		for _, e := range respBody.Errors {
			code := strconv.Itoa(e.Code)
			errMessage += "Error code " + code + ", " + e.Message + "."
		}
		return CloudflareRecord{}, errors.New(errMessage)
	}

	return respBody.Result, nil
}

//CreateTXTRecord - Create a txt record
func (c *Cloudflare) CreateTXTRecord(name, content string, ttl int, proxied bool) (record CloudflareRecord, err error) {
	newRecord := CloudflareRecordReq{
		RecordType: "TXT",
		Name:       name,
		Content:    content,
		TTL:        ttl,
		Proxied:    proxied,
	}
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(newRecord)

	c.mux.Lock()
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records"
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return CloudflareRecord{}, err
	}
	req.Header.Add("X-Auth-Email", c.AuthEmail)
	req.Header.Add("X-Auth-Key", c.AuthToken)
	req.Header.Add("Content-type", "application/json")
	c.mux.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return CloudflareRecord{}, err
	}
	defer resp.Body.Close() //Close the resp body when finished

	respBody := CloudflareSingleResp{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return CloudflareRecord{}, err
	}

	//Check if success, print errors from api if not.
	var errMessage string
	if !respBody.Success {
		for _, e := range respBody.Errors {
			code := strconv.Itoa(e.Code)
			errMessage += "Error code " + code + ", " + e.Message + "."
		}
		return CloudflareRecord{}, errors.New(errMessage)
	}

	return respBody.Result, nil
}

//UpdateRecordByID - Update record info
func (c *Cloudflare) UpdateRecordByID(record *CloudflareRecord) {

}

//PatchRecordByID - Patch record
func (c *Cloudflare) PatchRecordByID() {

}

//DeleteRecordByName - Delete record
func (c *Cloudflare) DeleteRecordByName(recordType, name string) error {
	record, err := c.GetRecord(recordType, name)
	if err != nil {
		return err
	}

	c.mux.Lock()
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneID + "/dns_records/" + record.ID
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("X-Auth-Email", c.AuthEmail)
	req.Header.Add("X-Auth-Key", c.AuthToken)
	req.Header.Add("Content-type", "application/json")
	c.mux.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //Close the resp body when finished

	respBody := CloudflareSingleResp{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return err
	}

	//Check if success, print errors from api if not.
	var errMessage string
	if !respBody.Success {
		for _, e := range respBody.Errors {
			code := strconv.Itoa(e.Code)
			errMessage += "Error code " + code + ", " + e.Message + "."
		}
		return errors.New(errMessage)
	}

	return nil
}
