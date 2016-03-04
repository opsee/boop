boop
=====

![boop](./boop.jpg)

A CLI for doing stuff to opsee, mostly bastion related

Installation
------------

    go get github.com/opsee/boop

Usage
-----

See:

    boop --help

(Ensure you are connected to the opsee VPN)

### List Bastions

    % boop bastion list "sterling@isis.com"
    d07cac86-df4a-11e5-a446-4b21b841f273 active 21s
    45cde7e8-d118-11e5-a310-ef438a026494 inactive 101h41m19s

### Reboot Bastions

    % boop bastion restart "sterling@isis.com" d07cac86-df4a-11e5-a446-4b21b841f273
    instance restart requested for: i-77a708b4 in us-west-1
