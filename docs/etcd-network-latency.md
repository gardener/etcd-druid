# Network Latency analysis: `sn-etcd-sz` vs  `mn-etcd-sz` vs `mn-etcd-mz`
This page captures the ETCD cluster latency analysis for below scenarios using [benchmark tool](benchmark) (build from [ETCD benchmark tool](https://github.com/seshachalam-yv/etcd)).

`sn-etcd-sz` -> single-node ETCD single zone (Only single replica of etcd will be running)

`mn-etcd-sz` -> multi-node ETCD single zone (Multiple replicas of etcd pods will be running across nodes in a single zone)

`mn-etcd-mz` -> multi-node ETCD multi zone (Multiple replicas of etcd pods will be running across nodes in multiple zones)

## PUT Analysis

### Summary 

* `sn-etcd-sz` latency is **~20% less than** `mn-etcd-sz` when benchmark tool with single client.
* `mn-etcd-sz` latency is less than `mn-etcd-mz` but the difference is `~+/-5%`.
* Compared to `mn-etcd-sz`, `sn-etcd-sz` latency is higher and gradually grows with more clients and larger value size. 
* Compared to `mn-etcd-mz`, `mn-etcd-sz` latency is higher and gradually grows with more clients and larger value size. 
* *Compared to `follower`, `leader` latency is less*, when benchmark tool with single client for all cases.
* *Compared to `follower`, `leader` latency is high*, when benchmark tool with multiple clients for all cases.


Sample commands:

```bash
# write to leader
benchmark put --target-leader --conns=1 --clients=1 --precise \
    --sequential-keys --key-starts 0 --val-size=256 --total=10000 \
    --endpoints=$ETCD_HOST 


# write to follower
benchmark put  --conns=1 --clients=1 --precise \
    --sequential-keys --key-starts 0 --val-size=256 --total=10000 \
    --endpoints=$ETCD_FOLLOWER_HOST

```

### Latency analysis during PUT requests to ETCD

* 
  <details>
  <summary>In this case benchmark tool tries to put key with random 256 bytes value.</summary>

  * Benchmark tool loads key/value to `leader` with single client .
    * `sn-etcd-sz` latency (~0.815ms) is **~50% lesser than** `mn-etcd-sz` (~1.74ms ).
    *  * `mn-etcd-sz` latency (~1.74ms ) is slightly lesser than `mn-etcd-mz` (~1.8ms) but the difference is negligible (within same ms).
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      10000     |     256    |           1           |         1         |       leader       |      1220.0520     |           0.815ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
      |      10000     |     256    |           1           |         1         |       leader       |       586.545      |            1.74ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
      |      10000     |     256    |           1           |         1         |       leader       |  554.0155654442634 |            1.8ms            | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool loads key/value to `follower` with single client.
    * `mn-etcd-sz` latency(`~2.2ms`) is **20% to 30% lesser than** `mn-etcd-mz`(`~2.7ms`).
    * *Compare to `follower`, `leader` has lower latency.*
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      10000     |     256    |           1           |         1         |     follower-1     |       445.743      |            2.23ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
      |      10000     |     256    |           1           |         1         |     follower-1     |  378.9366747610789 |            2.63ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |

      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      10000     |     256    |           1           |         1         |     follower-2     |       457.967      |            2.17ms           | eu-west-1a | etcd-main-2 | mn-etcd-sz |
      |      10000     |     256    |           1           |         1         |     follower-2     |  345.6586129825796 |            2.89ms           | eu-west-1b | etcd-main-2 | mn-etcd-mz |

  * Benchmark tool loads key/value to `leader` with multiple clients.
    * `sn-etcd-sz` latency(`~78.3ms`) is **~10% greater than**  `mn-etcd-sz`(`~71.81ms`).
    * `mn-etcd-sz` latency(`~71.81ms`) is less than `mn-etcd-mz`(`~72.5ms`) but the difference is negligible.
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |     100000     |     256    |          100          |        1000       |       leader       |      12638.905     |           78.32ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
      |     100000     |     256    |          100          |        1000       |       leader       |      13789.248     |           71.81ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
      |     100000     |     256    |          100          |        1000       |       leader       | 13728.446436395223 |            72.5ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |  



  * Benchmark tool loads key/value to `follower` with multiple clients.
    * `mn-etcd-sz` latency(`~69.8ms`) is **~5% greater than**  `mn-etcd-mz`(`~72.6ms`).
    * *Compare to `leader`, `follower` has lower latency*.
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |     100000     |     256    |          100          |        1000       |     follower-1     |      14271.983     |           69.80ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
      |     100000     |     256    |          100          |        1000       |     follower-1     |      13695.98      |           72.62ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |     100000     |     256    |          100          |        1000       |     follower-2     |      14325.436     |           69.47ms           | eu-west-1a | etcd-main-2 | mn-etcd-sz |
      |     100000     |     256    |          100          |        1000       |     follower-2     | 15750.409490407475 |            63.3ms           | eu-west-1b | etcd-main-2 | mn-etcd-mz |

  </details>

* 
  <details>  
  <summary>In this case benchmark tool tries to put key with random 1 MB value.</summary>

  * Benchmark tool loads key/value to `leader` with single client.
    * `sn-etcd-sz` latency(`~16.35ms`) is **~20% lesser than** `mn-etcd-sz`(`~20.64ms`).
    * `mn-etcd-sz` latency(`~20.64ms`) is less than `mn-etcd-mz`(`~21.08ms`) but the difference is negligible..
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |           1           |         1         |       leader       |       61.117       |           16.35ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
      |      1000      |   1000000  |           1           |         1         |       leader       |       48.416       |           20.64ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
      |      1000      |   1000000  |           1           |         1         |       leader       |  45.7517341664802  |           21.08ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool loads key/value withto `follower` single client.
    * `mn-etcd-sz` latency(`~23.10ms`) is **~10% greater than** `mn-etcd-mz`(`~21.8ms`).
    * *Compare to `follower`, `leader` has lower latency*.
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |           1           |         1         |     follower-1     |       43.261       |           23.10ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
      |      1000      |   1000000  |           1           |         1         |     follower-1     |  45.7517341664802  |            21.8ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |
      |      1000      |   1000000  |           1           |         1         |     follower-1     |        45.33       |           22.05ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |

      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |           1           |         1         |     follower-2     |       40.0518      |           24.95ms           | eu-west-1a | etcd-main-2 | mn-etcd-sz |
      |      1000      |   1000000  |           1           |         1         |     follower-2     |  43.28573155709838 |           23.09ms           | eu-west-1b | etcd-main-2 | mn-etcd-mz |
      |      1000      |   1000000  |           1           |         1         |     follower-2     |        45.92       |           21.76ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |
      |      1000      |   1000000  |           1           |         1         |     follower-2     |       35.5705      |            28.1ms           | eu-west-1b | etcd-main-2 | mn-etcd-mz |

  * Benchmark tool loads key/value to `leader` with multiple clients.
    * `sn-etcd-sz` latency(`~6.0375secs`) is **~30% greater than**  `mn-etcd-sz``~4.000secs`).
    * `mn-etcd-sz` latency(`~4.000secs`) is less than `mn-etcd-mz`(`~ 4.09secs`) but the difference is negligible.
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |          100          |        300        |       leader       |       55.373       |          6.0375secs         | eu-west-1c | etcd-main-0 | sn-etcd-sz |
      |      1000      |   1000000  |          100          |        300        |       leader       |       67.319       |          4.000secs          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
      |      1000      |   1000000  |          100          |        300        |       leader       |  65.91914167957594 |           4.09secs          | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool loads key/value to `follower` with multiple clients.
    * *`mn-etcd-sz` latency(`~4.04secs`) is **~5% greater than** `mn-etcd-mz`(`~ 3.90secs`).*
    * *Compare to `leader`, `follower` has lower latency*. 
    * 
      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |          100          |        300        |     follower-1     |       66.528       |          4.0417secs         | eu-west-1a | etcd-main-0 | mn-etcd-sz |
      |      1000      |   1000000  |          100          |        300        |     follower-1     |  70.6493461856332  |           3.90secs          | eu-west-1c | etcd-main-0 | mn-etcd-mz |
      |      1000      |   1000000  |          100          |        300        |     follower-1     |        71.95       |           3.84secs          | eu-west-1c | etcd-main-0 | mn-etcd-mz |

      | Number of keys | Value size | Number of connections | Number of clients | Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
      |:--------------:|:----------:|:---------------------:|:-----------------:|:------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
      |      1000      |   1000000  |          100          |        300        |     follower-2     |       66.447       |          4.0164secs         | eu-west-1a | etcd-main-2 | mn-etcd-sz |
      |      1000      |   1000000  |          100          |        300        |     follower-2     |  67.53038086369484 |           3.87secs          | eu-west-1b | etcd-main-2 | mn-etcd-mz |
      |      1000      |   1000000  |          100          |        300        |     follower-2     |        68.46       |           3.92secs          | eu-west-1a | etcd-main-1 | mn-etcd-mz |
  </details>


<hr>
<br>

## Range Analysis

Sample commands are:

```bash
# Single connection read request with sequential keys
benchmark range 0 --target-leader --conns=1 --clients=1 --precise \
    --sequential-keys --key-starts 0  --total=10000 \
    --consistency=l \
    --endpoints=$ETCD_HOST 
# --consistency=s [Serializable]
benchmark range 0 --target-leader --conns=1 --clients=1 --precise \
    --sequential-keys --key-starts 0  --total=10000 \
    --consistency=s \
    --endpoints=$ETCD_HOST 
# Each read request with range query matches key 0 9999 and repeats for total number of requests.  
benchmark range 0 9999 --target-leader --conns=1 --clients=1 --precise \
    --total=10 \
    --consistency=s \
    --endpoints=https://etcd-main-client:2379
# Read requests with multiple connections
benchmark range 0 --target-leader --conns=100 --clients=1000 --precise \
    --sequential-keys --key-starts 0  --total=100000 \
    --consistency=l \
    --endpoints=$ETCD_HOST 
benchmark range 0 --target-leader --conns=100 --clients=1000 --precise \
    --sequential-keys --key-starts 0  --total=100000 \
    --consistency=s \
    --endpoints=$ETCD_HOST 
```


### Latency analysis during Range requests to ETCD 

* 
  <details>
  <summary>In this case benchmark tool tries to get specific key with random 256 bytes value.</summary>
  
  * Benchmark tool range requests to `leader` with single client.

    * `sn-etcd-sz` latency(`~1.24ms`) is **~40% greater than** `mn-etcd-sz`(`~0.67ms`).
    * `mn-etcd-sz` latency(`~0.67ms`) is  **~20% lesser than** `mn-etcd-mz`(`~0.85ms`).
    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        10000       |     256    |           1           |         1         |       true      |      l      |        leader       |       800.272      |            1.24ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      l      |        leader       |      1173.9081     |            0.67ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      l      |        leader       |  999.3020189178693 |            0.85ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

    * Compare to consistency `Linearizable`, `Serializable` is **~40% less** for all cases
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        10000       |     256    |           1           |         1         |       true      |      s      |        leader       |      1411.229      |            0.70ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      s      |        leader       |      2033.131      |            0.35ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      s      |        leader       | 2100.2426362012025 |            0.47ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` with single client .
     * `mn-etcd-sz` latency(`~1.3ms`) is  **~20% lesser than** `mn-etcd-mz`(`~1.6ms`).
    * *Compare to `follower`, `leader` read request latency is **~50% less** for both `mn-etcd-sz`, `mn-etcd-mz`*
    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        10000       |     256    |           1           |         1         |       true      |      l      |      follower-1     |       765.325      |            1.3ms            | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      l      |      follower-1     |        596.1       |            1.6ms            | eu-west-1c | etcd-main-0 | mn-etcd-mz |
    * Compare to consistency `Linearizable`, `Serializable` is **~50% less** for all cases
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        10000       |     256    |           1           |         1         |       true      |      s      |      follower-1     |      1823.631      |            0.54ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        10000       |     256    |           1           |         1         |       true      |      s      |      follower-1     |       1442.6       |            0.69ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |
    |        10000       |     256    |           1           |         1         |       true      |      s      |      follower-1     |       1416.39      |            0.70ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |
    |        10000       |     256    |           1           |         1         |       true      |      s      |      follower-1     |      2077.449      |            0.47ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |
  
  * Benchmark tool range requests to `leader` with multiple client.
    * `sn-etcd-sz` latency(`~84.66ms`) is **~20% greater than** `mn-etcd-sz`(`~73.95ms`).
    * `mn-etcd-sz` latency(`~73.95ms`) is  **more or less equal to** `mn-etcd-mz`(`~ 73.8ms`).
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |       100000       |     256    |          100          |        1000       |       true      |      l      |        leader       |      11775.721     |           84.66ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      l      |        leader       |     13446.9598     |           73.95ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      l      |        leader       |  13527.19810605353 |            73.8ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |
    
    * Compare to consistency `Linearizable`, `Serializable` is **~20% lesser** for all cases
    * `sn-etcd-sz` latency(`~69.37ms`) is  **more or less equal to** `mn-etcd-sz`(`~69.89ms`).
    * `mn-etcd-sz` latency(`~69.89ms`) is  **slightly higher than** `mn-etcd-mz`(`~67.63ms`).
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |       100000       |     256    |          100          |        1000       |       true      |      s      |        leader       |     14334.9027     |           69.37ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      s      |        leader       |      14270.008     |           69.89ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      s      |        leader       | 14715.287354023869 |           67.63ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` with multiple client.
    * `mn-etcd-sz` latency(`~60.69ms`) is **~20% lesser than** `mn-etcd-mz`(`~70.76ms`).
    * Compare to  `leader`, `follower` has lower read request latency.
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |       100000       |     256    |          100          |        1000       |       true      |      l      |      follower-1     |      11586.032     |           60.69ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      l      |      follower-1     |       14050.5      |           70.76ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |


    * `mn-etcd-sz` latency(`~86.09ms`) is **~20 higher than** `mn-etcd-mz`(`~64.6ms`).
    * * Compare to `mn-etcd-sz` consistency `Linearizable`, `Serializable` is **~20% higher**.*
    *  Compare to `mn-etcd-mz` consistency `Linearizable`, `Serializable` is **~slightly less**.
    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |       100000       |     256    |          100          |        1000       |       true      |      s      |      follower-1     |      11582.438     |           86.09ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |       100000       |     256    |          100          |        1000       |       true      |      s      |      follower-1     |       15422.2      |            64.6ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |


  * Benchmark tool range requests to `leader` all keys.
    * `sn-etcd-sz` latency(`~678.77ms`) is **~5% slightly lesser than** `mn-etcd-sz`(`~697.29ms`).
    * `mn-etcd-sz` latency(`~697.29ms`) is less than `mn-etcd-mz`(`~701ms`) but the difference is negligible.
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |     256    |           2           |         5         |      false      |      l      |        leader       |       6.8875       |           678.77ms          | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      l      |        leader       |        6.720       |           697.29ms          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      l      |        leader       |         6.7        |            701ms            | eu-west-1a | etcd-main-1 | mn-etcd-mz |

    * * Compare to consistency `Linearizable`, `Serializable` is **~5% slightly higher** for all cases
    * `sn-etcd-sz` latency(`~687.36ms`) is less than `mn-etcd-sz`(`~692.68ms`) but the difference is negligible.
    * `mn-etcd-sz` latency(`~692.68ms`) is **~5% slightly lesser than** `mn-etcd-mz`(`~735.7ms`).
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |     256    |           2           |         5         |      false      |      s      |        leader       |        6.76        |           687.36ms          | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      s      |        leader       |        6.635       |           692.68ms          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      s      |        leader       |         6.3        |           735.7ms           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` all keys
    * `mn-etcd-sz`(`~737.68ms`) latency is **~5% slightly higher than** `mn-etcd-mz`(`~713.7ms`).
    * Compare to `leader` consistency `Linearizable`read request, `follower` is *~5% slightly higher*. 
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |     256    |           2           |         5         |      false      |      l      |      follower-1     |        6.163       |           737.68ms          | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      l      |      follower-1     |        6.52        |           713.7ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |

    * `mn-etcd-sz` latency(`~757.73ms`) is **~10% higher than** `mn-etcd-mz`(`~690.4ms`).
    * Compare to `follower` consistency `Linearizable`read request, `follower`  consistency `Serializable`  is *~3% slightly higher* for `mn-etcd-sz`.
    * *Compare to `follower` consistency `Linearizable`read request, `follower`  consistency `Serializable`  is *~5% less* for `mn-etcd-mz`.*
    * *Compare to `leader` consistency `Serializable`read request, `follower` consistency `Serializable` is *~5% less* for `mn-etcd-mz`. *
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |     256    |           2           |         5         |      false      |      s      |      follower-1     |       6.0295       |           757.73ms          | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |         20         |     256    |           2           |         5         |      false      |      s      |      follower-1     |        6.87        |           690.4ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |




  <hr>
  <br>
  </details>

* 
  <details>

  <summary>In this case benchmark tool tries to get specific key with random `1MB` value.</summary>
  
  * Benchmark tool range requests to `leader` with single client.

    * `sn-etcd-sz` latency(`~5.96ms`) is **~5% lesser than** `mn-etcd-sz`(`~6.28ms`).
    * `mn-etcd-sz` latency(`~6.28ms`) is  **~10% higher than** `mn-etcd-mz`(`~5.3ms`).
    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |           1           |         1         |       true      |      l      |        leader       |       167.381      |            5.96ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      l      |        leader       |       158.822      |            6.28ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      l      |        leader       |       187.94       |            5.3ms            | eu-west-1a | etcd-main-1 | mn-etcd-mz |
    
    * Compare to consistency `Linearizable`, `Serializable` is **~15% less** for  `sn-etcd-sz`, `mn-etcd-sz`, `mn-etcd-mz`
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |           1           |         1         |       true      |      s      |        leader       |       184.95       |           5.398ms           | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      s      |        leader       |       176.901      |            5.64ms           | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      s      |        leader       |       209.99       |            4.7ms            | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` with single client.
    * `mn-etcd-sz` latency(`~6.66ms`) is  **~10% higher than** `mn-etcd-mz`(`~6.16ms`).
    * *Compare to `leader`, `follower` read request latency is **~10% high** for `mn-etcd-sz`*
    * *Compare to `leader`, `follower` read request latency is **~20% high** for  `mn-etcd-mz`*
    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |           1           |         1         |       true      |      l      |      follower-1     |       150.680      |            6.66ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      l      |      follower-1     |       162.072      |            6.16ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |

    * Compare to consistency `Linearizable`, `Serializable` is **~15% less** for  `mn-etcd-sz`(`~5.84ms`), `mn-etcd-mz`(`~5.01ms`).
    * *Compare to `leader`, `follower` read request latency is **~5% slightly high** for `mn-etcd-sz`, `mn-etcd-mz`*
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |           1           |         1         |       true      |      s      |      follower-1     |       170.918      |            5.84ms           | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        1000        |   1000000  |           1           |         1         |       true      |      s      |      follower-1     |       199.01       |            5.01ms           | eu-west-1c | etcd-main-0 | mn-etcd-mz |



  * Benchmark tool range requests to `leader` with multiple clients.

    * `sn-etcd-sz` latency(`~1.593secs`) is **~20% lesser than** `mn-etcd-sz`(`~1.974secs`).
    * `mn-etcd-sz` latency(`~1.974secs`) is  **~5% greater than** `mn-etcd-mz`(`~1.81secs`).
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |        leader       |       252.149      |          1.593secs          | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |        leader       |       205.589      |          1.974secs          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |        leader       |       230.42       |           1.81secs          | eu-west-1a | etcd-main-1 | mn-etcd-mz |

   * *Compare to consistency `Linearizable`, `Serializable` is **more or less same** for `sn-etcd-sz`(`~1.57961secs`), `mn-etcd-mz`(`~1.8secs`) not a big difference*
   * Compare to consistency `Linearizable`, `Serializable` is  **~10% high** for `mn-etcd-sz`(`~ 2.277secs`).
   * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |        leader       |       252.406      |         1.57961secs         | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |        leader       |       181.905      |          2.277secs          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |        leader       |       227.64       |           1.8secs           | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` with multiple client.

    * `mn-etcd-sz` latency is **~20% less than** `mn-etcd-mz`.
    * Compare to  `leader` consistency `Linearizable`, `follower` read request latency is ~15 less for `mn-etcd-sz`(`~1.694secs`).    
    * Compare to  `leader` consistency `Linearizable`, `follower` read request latency is ~10% higher for `mn-etcd-sz`(`~1.977secs`).    
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |      follower-1     |       248.489      |          1.694secs          | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |      follower-1     |       210.22       |          1.977secs          | eu-west-1c | etcd-main-0 | mn-etcd-mz |



    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |      follower-2     |       205.765      |          1.967secs          | eu-west-1a | etcd-main-2 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      l      |      follower-2     |        195.2       |          2.159secs          | eu-west-1b | etcd-main-2 | mn-etcd-mz |


    *  
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |      follower-1     |       231.458      |          1.7413secs         | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |      follower-1     |       214.80       |          1.907secs          | eu-west-1c | etcd-main-0 | mn-etcd-mz |


    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |      follower-2     |       183.320      |          2.2810secs         | eu-west-1a | etcd-main-2 | mn-etcd-sz |
    |        1000        |   1000000  |          100          |        500        |       true      |      s      |      follower-2     |       195.40       |          2.164secs          | eu-west-1b | etcd-main-2 | mn-etcd-mz |



  * Benchmark tool range requests to `leader` all keys.

    * `sn-etcd-sz` latency(`~8.993secs`) is **~3% slightly lower than** `mn-etcd-sz`(`~9.236secs`).
    * `mn-etcd-sz` latency(`~9.236secs`) is  **~2% slightly lower than** `mn-etcd-mz`(`~9.100secs`).
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |   1000000  |           2           |         5         |      false      |      l      |        leader       |       0.5139       |          8.993secs          | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      l      |        leader       |        0.506       |          9.236secs          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      l      |        leader       |        0.508       |          9.100secs          | eu-west-1a | etcd-main-1 | mn-etcd-mz |
    
    * Compare to consistency `Linearizable`read request, `follower` for `sn-etcd-sz`(`~9.secs`) is **a slight difference `10ms`**.
    * Compare to consistency `Linearizable`read request, `follower` for `mn-etcd-sz`(`~9.113secs`) is **~1% less**, not a big difference.
    * Compare to consistency `Linearizable`read request, `follower` for `mn-etcd-mz`(`~8.799secs`) is **~3% less**, not a big difference.
    * `sn-etcd-sz` latency(`~9.secs`) is **~1% slightly less than** `mn-etcd-sz`(`~9.113secs`).
    * *`mn-etcd-sz` latency(`~9.113secs`) is  **~3% slightly higher than** `mn-etcd-mz`(`~8.799secs`)*.
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |   1000000  |           2           |         5         |      false      |      s      |        leader       |       0.51125      |          9.0003secs         | eu-west-1c | etcd-main-0 | sn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      s      |        leader       |       0.4993       |          9.113secs          | eu-west-1a | etcd-main-1 | mn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      s      |        leader       |        0.522       |          8.799secs          | eu-west-1a | etcd-main-1 | mn-etcd-mz |

  * Benchmark tool range requests to `follower` all keys

    * `mn-etcd-sz` latency(`~9.065secs`) is **~1% slightly higher than** `mn-etcd-mz`(`~9.007secs`).
    * Compare to `leader` consistency `Linearizable`read request, `follower` is *~1% slightly higher* for both cases  `mn-etcd-sz`,  `mn-etcd-mz` . 
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |   1000000  |           2           |         5         |      false      |      l      |      follower-1     |        0.512       |          9.065secs          | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      l      |      follower-1     |        0.533       |          9.007secs          | eu-west-1c | etcd-main-0 | mn-etcd-mz |


    * Compare to consistency `Linearizable`read request, `follower` for `mn-etcd-sz`(`~9.553secs`) is **~5% high**.
    * *Compare to consistency `Linearizable`read request, `follower` for `mn-etcd-mz`(`~7.7433secs`) is **~15% less***.

    * *`mn-etcd-sz`(`~9.553secs`) latency is  **~20% higher than** `mn-etcd-mz`(`~7.7433secs`)*.
    * 
    | Number of requests | Value size | Number of connections | Number of clients | sequential-keys | Consistency |  Target etcd server |  Average write QPS | Average latency per request |    zone    | server name |  Test name |
    |:------------------:|:----------:|:---------------------:|:-----------------:|:---------------:|:-----------:|:-------------------:|:------------------:|:---------------------------:|:----------:|:-----------:|:----------:|
    |         20         |   1000000  |           2           |         5         |      false      |      s      |      follower-1     |       0.4743       |          9.553secs          | eu-west-1a | etcd-main-0 | mn-etcd-sz |
    |         20         |   1000000  |           2           |         5         |      false      |      s      |      follower-1     |       0.5500       |          7.7433secs         | eu-west-1c | etcd-main-0 | mn-etcd-mz |

  <hr>
  <br>
  </details>


<hr>
<br>
<br>

>NOTE: This Network latency analysis is inspired by [ETCD performance](https://etcd.io/docs/v3.5/op-guide/performance/).
