package main

import (
	"fmt"

	"github.com/gagliardetto/treeout"
)

func main() {
	t := treeout.New("DEPLOYMENTS\nfoo")
	t.Child("dep1")
	t.Child("dep2")

	t2 := t.Child("dep3\nnewline\nnewline\nnewline\nnewline\nnewline")
	t2.Child("pod1").Child("pod1").Child("pod1\nnewline\nnewline\nnewline\nnewline\nnewline").Child("pod1").Child("pod1").Child("pod1").Child("pod1").Child("pod1").Child("pod1")
	t2.Child("pod2").Child("something")
	t2.Child("pod3").Child("something\nnewline\nnewline\nnewline\nnewline\nnewline")
	t2.Child("pod4\nnewline\nnewline\nnewline\nnewline\nnewline")

	t3 := t2.Child("pod5\nnewline\nnewline\nnewline\nnewline\nnewline")
	t3.Child("err: something happened\nmore info: here")
	t.Child("dep4").Child("child").Child("child").Child("child")

	t.Child("dep5").Child("child\nnewline\nnewline\nnewline\nnewline\nnewline").ParentFunc(func(b treeout.Branches) {
		b.Child("aaaa\nbbbbbb\nccccc")
	})

	fmt.Println(t)
}
