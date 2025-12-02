package bootstrap

import (
	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/fs"
	"github.com/OpenListTeam/OpenList/v4/internal/offline_download/tool"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/internal/setting"
	"github.com/OpenListTeam/OpenList/v4/internal/task"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/OpenListTeam/tache"
)

func taskFilterNegative(num int) int64 {
	if num < 0 {
		num = 0
	}
	return int64(num)
}

func syncTaskIndex[T task.TaskExtensionInfo](taskType string, manager task.Manager[T]) {
	if err := db.UpsertTaskRecordsFromTasks(taskType, manager.GetAll()); err != nil {
		utils.Log.Warnf("failed to sync task index for %s: %+v", taskType, err)
	}
}

func InitTaskManager() {
	fs.UploadTaskManager = tache.NewManager[*fs.UploadTask](tache.WithWorks(setting.GetInt(conf.TaskUploadThreadsNum, conf.Conf.Tasks.Upload.Workers)), tache.WithMaxRetry(conf.Conf.Tasks.Upload.MaxRetry)) //upload now indexed
	op.RegisterSettingChangingCallback(func() {
		fs.UploadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskUploadThreadsNum, conf.Conf.Tasks.Upload.Workers)))
	})
	fs.CopyTaskManager = tache.NewManager[*fs.FileTransferTask](tache.WithWorks(setting.GetInt(conf.TaskCopyThreadsNum, conf.Conf.Tasks.Copy.Workers)), tache.WithPersistFunction(db.GetTaskPersistReadFunc("copy", conf.Conf.Tasks.Copy.TaskPersistant), db.UpdateTaskDataAndIndexFunc[*fs.FileTransferTask]("copy", conf.Conf.Tasks.Copy.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Copy.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.CopyTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskCopyThreadsNum, conf.Conf.Tasks.Copy.Workers)))
	})
	fs.MoveTaskManager = tache.NewManager[*fs.FileTransferTask](tache.WithWorks(setting.GetInt(conf.TaskMoveThreadsNum, conf.Conf.Tasks.Move.Workers)), tache.WithPersistFunction(db.GetTaskPersistReadFunc("move", conf.Conf.Tasks.Move.TaskPersistant), db.UpdateTaskDataAndIndexFunc[*fs.FileTransferTask]("move", conf.Conf.Tasks.Move.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Move.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.MoveTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskMoveThreadsNum, conf.Conf.Tasks.Move.Workers)))
	})
	tool.DownloadTaskManager = tache.NewManager[*tool.DownloadTask](tache.WithWorks(setting.GetInt(conf.TaskOfflineDownloadThreadsNum, conf.Conf.Tasks.Download.Workers)), tache.WithPersistFunction(db.GetTaskPersistReadFunc("download", conf.Conf.Tasks.Download.TaskPersistant), db.UpdateTaskDataAndIndexFunc[*tool.DownloadTask]("download", conf.Conf.Tasks.Download.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Download.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		tool.DownloadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskOfflineDownloadThreadsNum, conf.Conf.Tasks.Download.Workers)))
	})
	tool.TransferTaskManager = tache.NewManager[*tool.TransferTask](tache.WithWorks(setting.GetInt(conf.TaskOfflineDownloadTransferThreadsNum, conf.Conf.Tasks.Transfer.Workers)), tache.WithPersistFunction(db.GetTaskPersistReadFunc("transfer", conf.Conf.Tasks.Transfer.TaskPersistant), db.UpdateTaskDataAndIndexFunc[*tool.TransferTask]("transfer", conf.Conf.Tasks.Transfer.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Transfer.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		tool.TransferTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskOfflineDownloadTransferThreadsNum, conf.Conf.Tasks.Transfer.Workers)))
	})
	if len(tool.TransferTaskManager.GetAll()) == 0 { //prevent offline downloaded files from being deleted
		CleanTempDir()
	}
	fs.ArchiveDownloadTaskManager = tache.NewManager[*fs.ArchiveDownloadTask](tache.WithWorks(setting.GetInt(conf.TaskDecompressDownloadThreadsNum, conf.Conf.Tasks.Decompress.Workers)), tache.WithPersistFunction(db.GetTaskPersistReadFunc("decompress", conf.Conf.Tasks.Decompress.TaskPersistant), db.UpdateTaskDataAndIndexFunc[*fs.ArchiveDownloadTask]("decompress", conf.Conf.Tasks.Decompress.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Decompress.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.ArchiveDownloadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskDecompressDownloadThreadsNum, conf.Conf.Tasks.Decompress.Workers)))
	})
	fs.ArchiveContentUploadTaskManager.Manager = tache.NewManager[*fs.ArchiveContentUploadTask](tache.WithWorks(setting.GetInt(conf.TaskDecompressUploadThreadsNum, conf.Conf.Tasks.DecompressUpload.Workers)), tache.WithMaxRetry(conf.Conf.Tasks.DecompressUpload.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.ArchiveContentUploadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskDecompressUploadThreadsNum, conf.Conf.Tasks.DecompressUpload.Workers)))
	})
	// sync existing tasks into index so first page reads fast
	syncTaskIndex("upload", fs.UploadTaskManager)
	syncTaskIndex("copy", fs.CopyTaskManager)
	syncTaskIndex("move", fs.MoveTaskManager)
	syncTaskIndex("download", tool.DownloadTaskManager)
	syncTaskIndex("transfer", tool.TransferTaskManager)
	syncTaskIndex("decompress", fs.ArchiveDownloadTaskManager)
	syncTaskIndex("decompress_upload", fs.ArchiveContentUploadTaskManager)
}
