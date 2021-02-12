# V2ray-sqlite

A tool to store and print traffic status into sqlite 3.  
Every time executed, this program will store the traffic status into the database and clear the records in v2ray. Then the program will print the traffic status in a table.

## Config

Config file is written in YAML. See the commands in comfig_sample.yml.

## Table Schema

`CREATE TABLE traffic(
user TEXT,
type TEXT,
value INTEGER,
PRIMARY KEY (user,type));`

## Arguments

- -reset : to reset the records in the database
- -config = FILE : path to config file
