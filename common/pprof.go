package common

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// Monitor 定时监控cpu使用率，超过阈值输出pprof文件
func Monitor() {
	// 防止监控协程异常导致进程退出
	defer func() {
		if r := recover(); r != nil {
			SysLog(fmt.Sprintf("pprof Monitor panic: %v", r))
		}
	}()
	var lastCPUProfile time.Time
	var lastMemProfile time.Time
	// 阈值与冷却时间
	cpuThreshold := 80.0
	memThreshold := 80.0
	cooldown := 3 * time.Minute
	var err error

	memThresholdStr := os.Getenv("MEM_THRESHOLD")
	if memThresholdStr != "" {
		memThreshold, err = strconv.ParseFloat(memThresholdStr, 64)
		if err != nil {
			SysLog("解析memThreshold失败 " + err.Error())
		}
	}
	cpuThresholdStr := os.Getenv("CPU_THRESHOLD")
	if cpuThresholdStr != "" {
		cpuThreshold, err = strconv.ParseFloat(cpuThresholdStr, 64)
		if err != nil {
			SysLog("解析cpuThreshold失败 " + err.Error())
		}
	}
	cooldownStr := os.Getenv("COOLDOWN")
	if cooldownStr != "" {
		cooldown, err = time.ParseDuration(cooldownStr)
		if err != nil {
			SysLog("解析cooldown失败 " + err.Error())
		}
	}
	for {
		percent, err := cpu.Percent(time.Second, false)
		if err != nil {
			SysLog("获取CPU使用率失败 " + err.Error())
			continue
		}

		if percent[0] > cpuThreshold && time.Since(lastCPUProfile) >= cooldown {
			fmt.Println("cpu usage too high")
			// write pprof file
			if _, err = os.Stat("./pprof"); os.IsNotExist(err) {
				err = os.Mkdir("./pprof", os.ModePerm)
				if err != nil {
					SysLog("创建pprof文件夹失败 " + err.Error())
					continue
				}
			}
			f, err1 := os.Create("./pprof/" + fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405")))
			if err1 != nil {
				SysLog("创建pprof文件失败 " + err1.Error())
				continue
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				SysLog("启动pprof失败 " + err.Error())
				continue
			}
			time.Sleep(10 * time.Second) // profile for 30 seconds
			pprof.StopCPUProfile()
			f.Close()
			lastCPUProfile = time.Now()
		}
		// 内存使用率超过阈值时，输出 heap pprof（带冷却时间）
		vm, err := mem.VirtualMemory()
		if err != nil {
			SysLog("获取内存信息失败 " + err.Error())
		} else if vm.UsedPercent > memThreshold && time.Since(lastMemProfile) >= cooldown {
			fmt.Println("memory usage too high")
			if _, err := os.Stat("./pprof"); os.IsNotExist(err) {
				if err := os.Mkdir("./pprof", os.ModePerm); err != nil {
					SysLog("创建pprof文件夹失败 " + err.Error())
				}
			}
			mf, err := os.Create("./pprof/" + fmt.Sprintf("mem-%s.pprof", time.Now().Format("20060102150405")))
			if err != nil {
				SysLog("创建pprof文件失败 " + err.Error())
			} else {
				// 触发一次 GC，让堆快照更准确
				runtime.GC()
				pprof.WriteHeapProfile(mf)
				mf.Close()
				lastMemProfile = time.Now()
			}
		}
		time.Sleep(30 * time.Second)
	}
}
