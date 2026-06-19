package main

import "io/ioutil"

func main() {
	_, _ = ioutil.ReadFile("/dev/null")
}
