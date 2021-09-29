# Unikraft Governance

This repository contains definitions, documentation and tools to facilate the governance of the Unikraft Open-Source project.

## Teams and SIGs

The Unikraft OSS project is organised on the principle that "everything-is-a-library," whether they are a wrapper library for an external OSS project or library, an architecture, platform or application.
There are also auxiliary repositories and projects which are part of the Unikraft OSS comunity which: aid or facilitate the construction of unikernels, such as with command-line companion tool [`kraft`](https://github.com/unikraft/kraft); forks of upstream libraries; or, anything else.
As a result, there are many repositories and directories within the [Unikraft core repository](https://github.com/unikraft/unikraft) which overlap and represent some interest or focus point.
To address the growing ecosystem, the Unikraft OSS project is organised into self-governing Special Interest Groups (SIGs).
Each SIG oversees some number of libraries, repositories or code and are in themselves responsible for maintaining and reviewing changes.
This means there are dedicated maintainers and dedicated reviewers for each SIG.

 > A list of all Special Interest Groups and their maintainers, reviewers and members can be found in [`teams/`](https://github.com/unikraft/governance/tree/main/teams).

GitHub is the primary SCM used by Unikraft and it offers management tooling for organising persons via the [teams feature](https://docs.github.com/en/organizations/organizing-members-into-teams/about-teams).
The provided tooling offers heirarchical team management, meaning there can exist a sub-team within an existing team.
This feature is utilised in order to create the separation between "maintainers" and "reviewers" whilst still being part of the same SIG.

Each SIG has its own team within the [Unikraft Github organisation](https://github.com/unikraft), identified with `sig-$NAME`.
This represents an outer-most team and `maintainers-$NAME` and `reviewers-$NAME` representing two sub-teams within `sig-$NAME`.
This means that when you look at at all the Unikraft teams on GitHub, it will create something like this:

```
unikraft
├── sig-alloc
│   ├── maintainers-alloc
│   └── reviewers-alloc
├── sig-test
│   ├── maintainers-test
│   └── reviewers-test
├── ...
└── sig-etc
    ├── maintainers-etc
    └── reviewers-etc
```

Members listed in the `maintainers-$NAME` and `reviewers-$NAME` sub-teams will also be members of the higher-order `sig-$NAME` team but with the corresponding GitHub roles of "maintainer" and "member", respectively.
Members of a `sig-$NAME` team which are neither maintainers nor reviewers will simply be listed with the corresponding GitHub "member" role of the `sig-$NAME` team.

The purpose of organising the teams in this way is to:

1. Allow maintainers and reviewers to have their own internal groups for discussion, and to use the available teams feature of GitHub for their desired purposes for these particular role types;
2. Allow for quick reference of the groups of people by their role in any `CODEOWNERS` file and in the CI/CD with the syntax `@sig-$NAME`, or `@maintainers-$NAME` or `@reviewers-$NAME`; and,
3. We can reference all members of the Special Interest Groups, whether maintainer, reviewer or simply as a member with the handle `@sig-$NAME`.

### Joining a SIG

To join a Special Interest Group, create a pull request on this repository and add new line to the `members:` directive within the relevant team's YAML file, e.g.:

```diff
+   - name: Your Name
+     github: yourusername
```

Please include a short description as why you would like to join the team in your PR.
