# Replacing regex matches in repositories that belong to a user

```terminal
go run main.go \
--github_token=token \
--repo_name_matches=codemod_test \
--github_user=poorlydefinedbehaviour \
--replace='(?i)\w+Something\w+:DoSomethingC'
```

# Replacing regex matches in repositories that belong to a organization

```terminal
go run main.go \
--github_token=token \
--repo_name_matches=codemod_test \
--github_org=my_org_name \
--replace='(?i)\w+Something\w+:DoSomethingC'
```
