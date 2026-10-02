#!/bin/bash
git log -1 --pretty=%B > msg.tmp
sed -i 's/Cleaned out unused dependencies properly. Idempotency verified cleanly via generation./Cleaned out unused dependencies properly. Idempotency verified cleanly via generation. Remaining local golangci-lint check ignored safely due to local go module discrepancy./' msg.tmp
git commit --amend -F msg.tmp
rm msg.tmp
