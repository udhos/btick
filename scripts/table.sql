
DB_REAL=1 DB_USER=tickets DB_PASS=tickets1 DB_HOST=host:8080 DB_NAME=tickets go run main.go

mysql -u tickets -ptickets1 -h host -P 8080 -D tickets

use tickets;

CREATE TABLE ticket_table (
   user VARCHAR(100) NOT NULL,
   ticket VARCHAR(100) NOT NULL,
   PRIMARY KEY (user)
);
