#!/bin/bash

zcat TestData/TestBook-GnuCash/TestBook.gnucash | go run main.go gnucash-to-ledger - > /tmp/got.dat

diff TestData/TestBook-Ledger/TestBook.ledger.dat /tmp/got.dat

if [[ $? -ne 0 ]];
then
	echo "ERROR: Test failed."
	exit 1
fi

exit 0
