# telegraf-exec-btrfs-status

This is a simple tool to extract btrfs status and output [Influx line
protocol](https://docs.influxdata.com/influxdb/cloud/reference/syntax/line-protocol/);
it is designed to be used with a [telegraf exec
plugin](https://github.com/influxdata/telegraf/tree/master/plugins/inputs/exec).
This parses the output of `btrfs device stats`, `btrfs filesystem usage --raw`,
and `btrfs scrub status -d` and has been developed against Ubuntu 20.04 with
btrfs-progs v5.4.1 and InfluxDB 1.x for generating compatible line protocol. At
this time it needs root permissions because `btrfs device stats` and `btrfs
scrub status -d` require elevated permissions.

## Reference Output

This is sample `btrfs device stats` output this tool expects to parse:

```
[/dev/loop11].write_io_errs    0
[/dev/loop11].read_io_errs     0
[/dev/loop11].flush_io_errs    0
[/dev/loop11].corruption_errs  6144
[/dev/loop11].generation_errs  0
[/dev/loop10].write_io_errs    0
[/dev/loop10].read_io_errs     0
[/dev/loop10].flush_io_errs    0
[/dev/loop10].corruption_errs  6144
[/dev/loop10].generation_errs  0
[/dev/loop9].write_io_errs    0
[/dev/loop9].read_io_errs     0
[/dev/loop9].flush_io_errs    0
[/dev/loop9].corruption_errs  12288
[/dev/loop9].generation_errs  0
[/dev/loop8].write_io_errs    0
[/dev/loop8].read_io_errs     0
[/dev/loop8].flush_io_errs    0
[/dev/loop8].corruption_errs  6144
[/dev/loop8].generation_errs  0
```

This is sample `btrfs filesystem usage --raw` output:

```
Overall:
    Device size:                        4294967296
    Device allocated:                   4290772992
    Device unallocated:                    4194304
    Device missing:                              0
    Used:                               3892543488
    Free (estimated):                            0      (min: 0)
    Data ratio:                               2.00
    Metadata ratio:                           2.00
    Global reserve:                        3407872      (used: 0)

Data,RAID10: Size:1944059904, Used:1944059904 (100.00%)
   /dev/loop11   972029952
   /dev/loop10   972029952
   /dev/loop9    972029952
   /dev/loop8    972029952

Metadata,RAID10: Size:134217728, Used:2195456 (1.64%)
   /dev/loop11    67108864
   /dev/loop10    67108864
   /dev/loop9     67108864
   /dev/loop8     67108864

System,RAID10: Size:67108864, Used:16384 (0.02%)
   /dev/loop11    33554432
   /dev/loop10    33554432
   /dev/loop9     33554432
   /dev/loop8     33554432

Unallocated:
   /dev/loop11     1048576
   /dev/loop10     1048576
   /dev/loop9      1048576
   /dev/loop8      1048576
```

This is sample `btrfs scrub status -d` output:

```
UUID:             cef9ac15-c24e-47d0-9b68-6144c7119560
scrub device /dev/loop11 (id 1) history
Scrub started:    Fri Feb 12 22:27:03 2021
Status:           finished
Duration:         0:00:01
Total to scrub:   1023.00MiB
Rate:             928.02MiB/s
Error summary:    csum=6144
  Corrected:      5120
  Uncorrectable:  1024
  Unverified:     0
scrub device /dev/loop10 (id 2) history
Scrub started:    Fri Feb 12 22:27:03 2021
Status:           finished
Duration:         0:00:01
Total to scrub:   1023.00MiB
Rate:             928.03MiB/s
Error summary:    csum=6144
  Corrected:      0
  Uncorrectable:  6144
  Unverified:     0
scrub device /dev/loop9 (id 3) history
Scrub started:    Fri Feb 12 22:27:03 2021
Status:           finished
Duration:         0:00:01
Total to scrub:   1023.00MiB
Rate:             928.09MiB/s
Error summary:    csum=6144
  Corrected:      5120
  Uncorrectable:  1024
  Unverified:     0
scrub device /dev/loop8 (id 4) history
Scrub started:    Fri Feb 12 22:27:03 2021
Status:           finished
Duration:         0:00:01
Total to scrub:   1023.00MiB
Rate:             928.08MiB/s
Error summary:    csum=6144
  Corrected:      0
  Uncorrectable:  6144
  Unverified:     0
```

## Inreractive Run Example

The compiled tool can be run interactively. It assumes by default that the
[TextFSM](https://github.com/google/textfsm/wiki/TextFSM) [device stats
template](./btrfs_device_stats_template.txt), the [filesystem usage
template](./btrfs_filesystem_usage_template.txt), and the [scrub status
template](./btrfs_scrub_status_template.txt) are in your current directory, but
those can be set with the `--template-device-stats`,
`template-filesystem-usage`, and `template-scrub-status` CLI options.

```
sudo ./telegraf-exec-btrfs-status
btrfs_device_errors,device=/dev/loop10,mount=/mnt/test1 corruption_io_errors=6144i,flush_io_errors=0i,generation_io_errors=0i,read_io_errors=0i,write_io_errors=0i 1613960549025730071
btrfs_device_errors,device=/dev/loop11,mount=/mnt/test1 corruption_io_errors=6144i,flush_io_errors=0i,generation_io_errors=0i,read_io_errors=0i,write_io_errors=0i 1613960549025730071
btrfs_device_errors,device=/dev/loop8,mount=/mnt/test1 corruption_io_errors=6144i,flush_io_errors=0i,generation_io_errors=0i,read_io_errors=0i,write_io_errors=0i 1613960549025730071
btrfs_device_errors,device=/dev/loop9,mount=/mnt/test1 corruption_io_errors=12288i,flush_io_errors=0i,generation_io_errors=0i,read_io_errors=0i,write_io_errors=0i 1613960549025730071
btrfs_filesystem,aspect=Data,device=/dev/loop10,mount=/mnt/test1 device_size=972029952i 1613960549027613974
btrfs_filesystem,aspect=Data,device=/dev/loop11,mount=/mnt/test1 device_size=972029952i 1613960549027613974
btrfs_filesystem,aspect=Data,device=/dev/loop8,mount=/mnt/test1 device_size=972029952i 1613960549027613974
btrfs_filesystem,aspect=Data,device=/dev/loop9,mount=/mnt/test1 device_size=972029952i 1613960549027613974
btrfs_filesystem,aspect=Data,mount=/mnt/test1,type=RAID10 filesystem_size=1944059904i,filesystem_used=1944059904i,filesystem_used_percent=100 1613960549027613974
btrfs_filesystem,aspect=Metadata,device=/dev/loop10,mount=/mnt/test1 device_size=67108864i 1613960549027613974
btrfs_filesystem,aspect=Metadata,device=/dev/loop11,mount=/mnt/test1 device_size=67108864i 1613960549027613974
btrfs_filesystem,aspect=Metadata,device=/dev/loop8,mount=/mnt/test1 device_size=67108864i 1613960549027613974
btrfs_filesystem,aspect=Metadata,device=/dev/loop9,mount=/mnt/test1 device_size=67108864i 1613960549027613974
btrfs_filesystem,aspect=Metadata,mount=/mnt/test1,type=RAID10 filesystem_size=134217728i,filesystem_used=2195456i,filesystem_used_percent=1.6399999856948853 1613960549027613974
btrfs_filesystem,aspect=Overall,mount=/mnt/test1 filesystem_allocated=4290772992i,filesystem_data_ratio=2,filesystem_free_estimated=0i,filesystem_free_estimated_min=0i,filesystem_global_reserve=3407872i,filesystem_global_reserve_used=0i,filesystem_metadata_ratio=2,filesystem_missing=0i,filesystem_size=4294967296i,filesystem_unallocated=4194304i,filesystem_used=3892543488i 1613960549027613974
btrfs_filesystem,aspect=System,device=/dev/loop10,mount=/mnt/test1 device_size=33554432i 1613960549027613974
btrfs_filesystem,aspect=System,device=/dev/loop11,mount=/mnt/test1 device_size=33554432i 1613960549027613974
btrfs_filesystem,aspect=System,device=/dev/loop8,mount=/mnt/test1 device_size=33554432i 1613960549027613974
btrfs_filesystem,aspect=System,device=/dev/loop9,mount=/mnt/test1 device_size=33554432i 1613960549027613974
btrfs_filesystem,aspect=System,mount=/mnt/test1,type=RAID10 filesystem_size=67108864i,filesystem_used=16384i,filesystem_used_percent=0.019999999552965164 1613960549027613974
btrfs_filesystem,aspect=Unallocated,device=/dev/loop10,mount=/mnt/test1 device_size=1048576i 1613960549027613974
btrfs_filesystem,aspect=Unallocated,device=/dev/loop11,mount=/mnt/test1 device_size=1048576i 1613960549027613974
btrfs_filesystem,aspect=Unallocated,device=/dev/loop8,mount=/mnt/test1 device_size=1048576i 1613960549027613974
btrfs_filesystem,aspect=Unallocated,device=/dev/loop9,mount=/mnt/test1 device_size=1048576i 1613960549027613974
btrfs_scrub,device=/dev/loop10,device_id=2,mount=/mnt/test1 checksum_errors=6144i,corrected_errors=0i,duration=1i,rate=973109985u,start=1613190423i,status="finished",total=1072693248u,uncorrectable_errors=6144i,unverified_errors=0i 1613960549031873991
btrfs_scrub,device=/dev/loop11,device_id=1,mount=/mnt/test1 checksum_errors=6144i,corrected_errors=5120i,duration=1i,rate=973099499u,start=1613190423i,status="finished",total=1072693248u,uncorrectable_errors=1024i,unverified_errors=0i 1613960549031873991
btrfs_scrub,device=/dev/loop8,device_id=4,mount=/mnt/test1 checksum_errors=6144i,corrected_errors=0i,duration=1i,rate=973162414u,start=1613190423i,status="finished",total=1072693248u,uncorrectable_errors=6144i,unverified_errors=0i 1613960549031873991
btrfs_scrub,device=/dev/loop9,device_id=3,mount=/mnt/test1 checksum_errors=6144i,corrected_errors=5120i,duration=1i,rate=973172899u,start=1613190423i,status="finished",total=1072693248u,uncorrectable_errors=1024i,unverified_errors=0i 1613960549031873991
```

## Telegraf Run Example

This is a sample telegraf exec input that assumes the binary has been installed
to `/usr/local/bin/telegraf-exec-btrfs-status` and the TextFSM templates to
`/etc/telegraf/`:

```
[[inputs.exec]]                                                                 
  commands = ["/usr/local/bin/telegraf-exec-btrfs-status --template-device-stats=/etc/telegraf/btrfs_device_stats_template.txt --template-filesystem-usage=/etc/telegraf/btrfs_filesystem_usage_template.txt --template-scrub-status=/etc/telegraf/btrfs_scrub_status_template.txt"]
  timeout = "5s"                                                                
  data_format = "influx"      
```

Then in InfluxDB:

```
> show field keys from btrfs_device_errors
name: btrfs_device_errors
fieldKey             fieldType
--------             ---------
corruption_io_errors integer
flush_io_errors      integer
generation_io_errors integer
read_io_errors       integer
write_io_errors      integer
> show tag keys from btrfs_device_errors
name: btrfs_device_errors
tagKey
------
device
host
mount
```

```
> show field keys from btrfs_filesystem
name: btrfs_filesystem
fieldKey                       fieldType
--------                       ---------
device_size                    integer
filesystem_allocated           integer
filesystem_data_ratio          float
filesystem_free_estimated      integer
filesystem_free_estimated_min  integer
filesystem_global_reserve      integer
filesystem_global_reserve_used integer
filesystem_metadata_ratio      float
filesystem_missing             integer
filesystem_size                integer
filesystem_unallocated         integer
filesystem_used                integer
filesystem_used_percent        float
> show tag keys from btrfs_filesystem
name: btrfs_filesystem
tagKey
------
aspect
device
host
mount
type
```

```
> show field keys from btrfs_scrub
name: btrfs_scrub
fieldKey             fieldType
--------             ---------
checksum_errors      integer
corrected_errors     integer
duration             integer
rate                 integer
start                integer
status               string
total                integer
uncorrectable_errors integer
unverified_errors    integer
> show tag keys from btrfs_scrub
name: btrfs_scrub
tagKey
------
device
device_id
host
mount
```

# Future Work

Tests should be added especially considering the sensitivity of parsing.
