package worker

import(
	"github.com/c9s/goprocinfo/linux"
	"log"
)

type Stats struct {
	MemStats *linux.MemInfo
	DiskStats *linux.Disk
	CpuStats *linux.CPUStat
	LoadStats *linux.LoadAvg
	TaskCount int
}

func(s *Stats) MemTotalKb()uint64{
	return s.MemStats.MemTotal
}

func(s *Stats) MemAvailableKb() float64{
	return float64(s.MemStats.MemAvailable)
}

func(s *Stats) MemUsedKb() uint64{
	return s.MemStats.MemTotal- s.MemStats.MemAvailable
}

func(s *Stats) MemUsedPercent() uint64{
	return s.MemStats.MemAvailable/s.MemStats.MemTotal
}

func(s *Stats) DiskTotal() float64{
	return float64(s.DiskStats.All)
}

func(s *Stats) DiskFree() float64{
	return float64(s.DiskStats.Free)
}

func(s *Stats) DiskUsed() float64{
	return float64(s.DiskStats.Used)
}

func(s *Stats) CpuAvailable() float64{
	idle:=s.CpuStats.Idle + s.CpuStats.IOWait
	return float64(idle)
}

func(s *Stats) CpuUsage() float64{
	idle:=s.CpuStats.Idle + s.CpuStats.IOWait
	nonIdle:=s.CpuStats.User + s.CpuStats.Nice + s.CpuStats.System + s.CpuStats.IRQ + s.CpuStats.SoftIRQ + s.CpuStats.Steal
	total:=idle + nonIdle
	if total==0{
		return 0.0
	}
	return (float64(total)-float64(idle))/float64(total)
}

func GetStats() *Stats {
    mem := GetMemoryInfo()
    disk := GetDiskInfo()
    cpu := GetCpuStats()
    load := GetLoadAvg()

    stats := &Stats{
        MemStats:  mem,
        DiskStats: disk,
        CpuStats:  cpu,
        LoadStats: load,
    }

    return stats
}


func GetMemoryInfo() *linux.MemInfo{
	memStats,err:=linux.ReadMemInfo("/proc/meminfo")
	if err!=nil{
		log.Printf("Error reading from /proc/meminfo")
		return &linux.MemInfo{}
	}
	return memStats
}

func GetCpuStats() *linux.CPUStat{
	cpuStat,err:=linux.ReadStat("/proc/stat")
	if err!=nil{
		log.Printf("Error reading from /proc/stat")
		return &linux.CPUStat{}
	}
	return &cpuStat.CPUStatAll
}

func GetLoadAvg() *linux.LoadAvg{
	loadAvg,err:=linux.ReadLoadAvg("/proc/loadavg")
	if err!=nil {
		log.Printf("Error reading from /proc/loadavg ")
		return &linux.LoadAvg{}
	}
	return loadAvg
}

func GetDiskInfo() *linux.Disk {
	diskStats,err:=linux.ReadDisk("/")
	if err!=nil {
		log.Printf("Error reading from /")
		return &linux.Disk{}
	}
	return diskStats
}