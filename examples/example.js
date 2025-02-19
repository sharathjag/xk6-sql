import sql from "k6/x/sql";

// the actual database driver should be used instead of ramsql
import driver from "k6/x/sql/driver/ramsql";

export const options = {
  vus: 10,
};

// const conn_options = new sql.connOptions("3s", "3s", 1, 1);
const conn_options = new sql.connOptions({
  ConnMaxLifetime: "3s",
  ConnMaxIdleTime: "3s",
});
const db = sql.openWithOptions(driver, "roster_db", conn_options);
const query_timeout = sql.timeoutContext("3s")

export function setup() {
  db.execContext(query_timeout, `
    CREATE TABLE IF NOT EXISTS roster
      (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        given_name VARCHAR NOT NULL,
        family_name VARCHAR NOT NULL
      );
  `);
}

export function teardown() {
  db.close();
}

export default function () {
  let result = db.execContext(query_timeout, `
    INSERT INTO roster
      (given_name, family_name)
    VALUES
      ('Peter', 'Pan'),
      ('Wendy', 'Darling'),
      ('Tinker', 'Bell'),
      ('James', 'Hook');
  `);
  console.log(`${result.rowsAffected()} rows inserted`);

  let rows = db.queryContext(query_timeout, "SELECT * FROM roster WHERE given_name = $1;", "Peter");
  for (const row of rows) {
    console.log(`${row.family_name}, ${row.given_name}`);
  }
}
