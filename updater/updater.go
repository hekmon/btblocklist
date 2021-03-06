package updater

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hekmon/cunits/v2"
)

const (
	timeFormat = "2 Jan 2006 15:04:05 MST"
)

func (c *Controller) updater() {
	ticker := time.NewTicker(c.frequency)
	// Fist batch
	c.updaterBatch()
	// Next ones
	for {
		select {
		case <-ticker.C:
			c.updaterBatch()
		case <-c.ctx.Done():
			c.logger.Debug("[Updater] worker received stop signal")
			ticker.Stop()
			return
		}
	}
}

func (c *Controller) updaterBatch() {
	c.logger.Debug("[Updater] worker: starting a new batch")
	batchStart := time.Now()
	defer func() {
		c.lastBatch = batchStart
		var externalLines int
		for _, lines := range c.externalStates {
			externalLines += len(lines)
		}
		c.updateStatus(len(c.ripeState), len(c.externalStates), externalLines, c.lastUpdate, c.lastBatch)
	}()
	// Probing
	ripeUpdate := c.updateRipe()
	var externalUpdate bool
	for name, url := range c.blocklists {
		if c.getExternalBlockList(name, url.String()) {
			externalUpdate = true
		}
	}
	// Global update
	if !ripeUpdate && !externalUpdate {
		c.logger.Info("[Updater] No new data, keeping cache")
		return
	}
	data := c.compileFinalDataBlobFromCache()
	if data == nil {
		return
	}
	// Update the current data
	c.compressedDataAccess.Lock()
	c.compressedData = data
	c.compressedDataAccess.Unlock()
	c.lastUpdate = batchStart
	c.logger.Debug("[Updater] global cache updated")
}

func (c *Controller) compileFinalDataBlobFromCache() (data []byte) {
	startCompress := time.Now()
	c.logger.Info("[Updater] Merging and compressing all cached results")
	// Prepare the compressor
	compressed := bytes.NewBuffer(nil)
	compressor, err := gzip.NewWriterLevel(compressed, gzip.BestCompression)
	if err != nil {
		c.logger.Errorf("[Updater] Can't create the gzip compressor: %v", err)
		return
	}
	// Add the ripe data
	if _, err = compressor.Write([]byte("# BTBlocklist RIPE search\n")); err != nil {
		c.logger.Errorf("[Updater] Can't write RIPE search header: %v", err)
		return
	}
	ripeReader := bytes.NewBufferString(strings.Join(c.ripeState, "\n"))
	if _, err = io.Copy(compressor, ripeReader); err != nil {
		c.logger.Errorf("[Updater] Can't copy ripe results to the compressor: %v", err)
		return
	}
	if _, err = compressor.Write([]byte("\n")); err != nil {
		c.logger.Errorf("[Updater] Can't add \\n after RIPE results: %v", err)
		return
	}
	// Add the external data
	var externalLines int
	for name, lines := range c.externalStates {
		externalLines += len(lines)
		externalReader := bytes.NewBufferString(strings.Join(lines, "\n"))
		if _, err = io.Copy(compressor, externalReader); err != nil {
			c.logger.Errorf("[Updater] Can't copy '%s' results to the compressor: %v", name, err)
			return
		}
		if _, err = compressor.Write([]byte("\n")); err != nil {
			c.logger.Errorf("[Updater] Can't add \\n after '%s' results: %v", name, err)
			return
		}
	}
	// Finalize
	if err = compressor.Close(); err != nil {
		c.logger.Errorf("[Updater] Can't flush remaining bytes from the gzip compressor: %v", err)
		return
	}
	data = compressed.Bytes()
	c.logger.Infof("[Updater] %d range(s) from RIPE search and %d line(s) from %d external blocklist(s) compressed to %s in %v",
		len(c.ripeState), externalLines, len(c.externalStates), cunits.ImportInByte(float64(len(data))), time.Since(startCompress))
	return
}

func (c *Controller) updateStatus(nbRIPEranges, nbLists, nbLines int, lstModif, lstBatch time.Time) {
	if err := c.statusUpdate(fmt.Sprintf("RIPE: %d range(s) | External: %d list(s) with a total of %d line(s) | Last modification: %s | Last update: %s",
		nbRIPEranges, nbLists, nbLines, lstModif.Format(timeFormat), lstBatch.Format(timeFormat))); err != nil {
		c.logger.Errorf("[Updater] can't update status msg: %v", err)
	}
}
