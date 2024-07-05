# liquix

## SQL转换

1. createChangelog 比对两个指定数据库中的表结构，生成相应的 changelog.xml 文件
```shell
./liquibase --changeLogFile=changelog/changelog-ddl.xml --defaultsFile=config/diff.properties diffChangeLog
```

2. convertChangelog 根据 dbType 指定的数据类型，将 changelog.xml 转换成对应的 sql 脚本
```shell
# mysql
./liquibase --changeLogFile=changelog/changelog-ddl.xml --defaultsFile=config/mysql.properties updateSql

# sqlserver
./liquibase --changeLogFile=changelog/changelog-ddl.xml --defaultsFile=config/sqlserver.properties updateSql

# oracle
./liquibase --changeLogFile=changelog/changelog-ddl.xml --defaultsFile=config/oracle.properties updateSql
```
