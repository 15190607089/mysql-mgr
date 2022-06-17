/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	v14 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	publicappv1 "mysqlmgr/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MysqlReconciler reconciles a Mysql object
type MysqlReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=publicapp.emergen.cn,resources=mysqls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=publicapp.emergen.cn,resources=mysqls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=publicapp.emergen.cn,resources=mysqls/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Mysql object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *MysqlReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	logger := r.Log.WithValues("mysql", req.NamespacedName)
	logger.Info("start reconcile")
	// TODO(user): your logic here
	instance := &publicappv1.Mysql{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	//statefulset逻辑
	statefulSet := &v1.StatefulSet{}
	statful := newStatful(instance)
	err := controllerutil.SetControllerReference(instance, statful, r.Scheme)
	if err != nil {
		logger.Info("scale up failed: setcontrollerreference")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, statefulSet); errors.IsNotFound(err) {
		logger.Info("found none statefulset,create now")
		if err := r.Create(ctx, statful); err != nil {
			logger.Error(err, "create statefulset failed")
			return ctrl.Result{}, err
		}
	} else if err == nil {
		if err := r.Update(ctx, statful); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, err
	}
	//configmap逻辑
	configMap := &v14.ConfigMap{}
	cm := newconfigmap(instance)
	if err := controllerutil.SetControllerReference(instance, cm, r.Scheme); err != nil {
		logger.Info("scale up failed: setcontrollerreference")
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, configMap); errors.IsNotFound(err) {
		logger.Info("found none configmap,create now")
		if err := r.Create(ctx, cm); err != nil {
			logger.Error(err, "create configmap failed")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}
	//service逻辑
	service := &v14.Service{}
	svc := newsvc(instance)
	if err := controllerutil.SetControllerReference(instance, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, service); errors.IsNotFound(err) {
		logger.Info("found none service,create now")
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "create service failed")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func newconfigmap(cr *publicappv1.Mysql) *v14.ConfigMap {
	return &v14.ConfigMap{
		ObjectMeta: v12.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    map[string]string{"app": cr.Name},
		},
		Data: map[string]string{
			"master.cnf": "# Apply this config only on the master.\n[mysqld]\nlog-bin\nsecure_file_priv=/var/lib/mysql\ntransaction_isolation = READ-COMMITTED\nlog_timestamps=system\nrelay_log_recovery=ON\nmax_connections=2000\nmax_connect_errors=5000\nwait_timeout=864000\ninteractive_timeout=864000\nmax_allowed_packet=100M\ndefault_authentication_plugin=mysql_native_password\nthread_cache_size=64\ninnodb_buffer_pool_size=15G\nread_rnd_buffer_size=16M\nbulk_insert_buffer_size=100M\nsort_buffer_size=16M\njoin_buffer_size=16M\ninnodb_read_io_threads=6\ninnodb_write_io_threads=6\nread_only=0\ninnodb_force_recovery=0\ndefault-time_zone = '+8:00'\n[mysql]\ndefault-character-set = utf8mb4\n[client]\ndefault-character-set = utf8mb4",
			"slave.cnf":  "# Apply this config only on slaves.\nsuper-read-only\nsecure_file_priv=/var/lib/mysql\ntransaction_isolation = READ-COMMITTED\nlog_timestamps=system\nrelay_log_recovery=ON\nmax_connections=2000\nmax_connect_errors=5000\nwait_timeout=864000\ninteractive_timeout=864000\nmax_allowed_packet=100M\ndefault_authentication_plugin=mysql_native_password\nthread_cache_size=64\ninnodb_buffer_pool_size=15G\nread_buffer_size=32M\nread_rnd_buffer_size=16M\nbulk_insert_buffer_size=100M\nsort_buffer_size=16M\njoin_buffer_size=16M\ninnodb_read_io_threads=6\ninnodb_write_io_threads=6\nread_only=0\ninnodb_force_recovery=0\ndefault-time_zone = '+8:00'\n[mysql]\ndefault-character-set = utf8mb4\n[client]\ndefault-character-set = utf8mb4",
		},
	}
}

func newsvc(cr *publicappv1.Mysql) *v14.Service {
	return &v14.Service{
		ObjectMeta: v12.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    map[string]string{"app": cr.Name},
		},
		Spec: v14.ServiceSpec{
			Ports: []v14.ServicePort{
				{
					Name: "mysql",
					Port: 3306,
				},
			},
			ClusterIP: "None",
			Selector:  map[string]string{"app": cr.Name},
		},
	}
}

func newStatful(cr *publicappv1.Mysql) *v1.StatefulSet {
	lbs := map[string]string{"app": cr.Name}

	scn := cr.Spec.Storageclass
	repls := int32(cr.Spec.Replicas)
	return &v1.StatefulSet{
		ObjectMeta: v12.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
			Labels:    lbs,
		},
		Spec: v1.StatefulSetSpec{
			ServiceName: cr.Name,
			Replicas:    &repls,
			Selector:    &v12.LabelSelector{MatchLabels: map[string]string{"app": cr.Name}},
			Template: v14.PodTemplateSpec{
				ObjectMeta: v12.ObjectMeta{
					Labels: lbs,
				},
				Spec: v14.PodSpec{
					Affinity: &v14.Affinity{
						PodAntiAffinity: &v14.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v14.PodAffinityTerm{
								{
									LabelSelector: &v12.LabelSelector{
										MatchExpressions: []v12.LabelSelectorRequirement{
											{
												Key:      "app",
												Operator: v12.LabelSelectorOpIn,
												Values: []string{
													cr.Name,
												},
											},
										},
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
					Tolerations: []v14.Toleration{
						{
							Key:      "app",
							Operator: v14.TolerationOpEqual,
							Value:    "mysql",
							Effect:   v14.TaintEffectNoSchedule,
						},
					},
					InitContainers: []v14.Container{
						{
							Name:  "init-mysql",
							Image: "mysql:8.0.18",
							Command: []string{
								"bash",
								"-c",
								"set ex\n# 从hostname中获取索引，比如(mysql-1)会获取(1)\n[[ `hostname` =~ -([0-9]+)$ ]] || exit 1\nordinal=${BASH_REMATCH[1]}\necho [mysqld] > /mnt/conf.d/server-id.cnf\n          # 为了不让server-id=0而增加偏移量\necho server-id=$((100 + $ordinal)) >> /mnt/conf.d/server-id.cnf\n# 拷贝对应的文件到/mnt/conf.d/文件夹中\nif [[ $ordinal -eq 0 ]]; then\n  cp /mnt/config-map/master.cnf /mnt/conf.d/\nelse\n  cp /mnt/config-map/slave.cnf /mnt/conf.d/\nfi",
							},
							VolumeMounts: []v14.VolumeMount{
								{
									Name:      "conf",
									MountPath: "/mnt/conf.d",
								},
								{
									Name:      "config-map",
									MountPath: "/mnt/config-map",
								},
							},
						},
						{
							Name:  "clone-mysql",
							Image: "jstang/xtrabackup:2.3",
							Command: []string{
								"bash",
								"-c",
								"set -ex\n# 整体意思:\n# 1.如果是主mysql中的xtrabackup,就不需要克隆自己了,直接退出\n# 2.如果是从mysql中的xtrabackup,先判断是否是第一次创建，因为第二次重启本地就有数据库，无需克隆。若是第一次创建(通过/var/lib/mysql/mysql文件是否存在判断),就需要克隆数据库到本地。\n# 如果有数据不必克隆数据，直接退出()\n[[ -d /var/lib/mysql/mysql ]] && exit 0\n# 如果是master数据也不必克隆\n[[ `hostname` =~ -([0-9]+)$ ]] || exit 1\nordinal=${BASH_REMATCH[1]}\n[[ $ordinal -eq 0 ]] && exit 0\n# 从序列号比自己小一的数据库克隆数据，比如mysql-2会从mysql-1处克隆数据\nncat --recv-only mysql-$(($ordinal-1)).mysql 3307 | xbstream -x -C /var/lib/mysql\n# 比较数据\nxtrabackup --prepare --target-dir=/var/lib/mysql",
							},
							VolumeMounts: []v14.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/mysql",
									SubPath:   "mysql",
								},
								{
									Name:      "conf",
									MountPath: "/etc/mysql/conf.d",
								},
							},
						},
					},
					Containers: []v14.Container{
						{
							Name:  "mysql",
							Image: "mysql:8.0.18",
							Args: []string{
								"--default-authentication-plugin=mysql_native_password",
							},
							Env: []v14.EnvVar{
								{
									Name:  "MYSQL_ALLOW_EMPTY_PASSWORD",
									Value: "1",
								},
							},
							Ports: []v14.ContainerPort{
								{
									Name:          "mysql",
									ContainerPort: 3306,
								},
							},
							VolumeMounts: []v14.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/mysql",
									SubPath:   "mysql",
								},
								{
									Name:      "conf",
									MountPath: "/etc/mysql/conf.d",
								},
							},
							Resources: v14.ResourceRequirements{
								Requests: v14.ResourceList{
									v14.ResourceCPU:    resource.MustParse("50m"),
									v14.ResourceMemory: resource.MustParse("50Mi"),
								},
							},
							LivenessProbe: &v14.Probe{
								Handler: v14.Handler{
									Exec: &v14.ExecAction{
										Command: []string{
											"mysqladmin",
											"ping",
										},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
							ReadinessProbe: &v14.Probe{
								Handler: v14.Handler{
									Exec: &v14.ExecAction{
										Command: []string{
											"mysql",
											"-h",
											"127.0.0.1",
											"-e",
											"SELECT 1",
										},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       2,
								TimeoutSeconds:      1,
							},
						},
						{
							Name:  "xtrabackup",
							Image: "jstang/xtrabackup:2.3",
							Ports: []v14.ContainerPort{
								{
									Name:          "xtrabackup",
									ContainerPort: 3307,
								},
							},
							Command: []string{
								"bash",
								"-c",
								"set -ex\n          hostname=`hostname`\n          tmpuser=`mysql -h 127.0.0.1 -uroot -p -e \"select user from mysql.user where user='repl' limit  1;\"`\n          if [[ $hostname = mysql-0 ]];then\n            if [[ ! $tmpuser ]];then\n              mysql -h 127.0.0.1 -uroot -p -e \"CREATE USER 'repl'@'%' IDENTIFIED BY 'repl' ;\"\n              mysql -h 127.0.0.1 -uroot -p -e \"GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%' ;\"\n              mysql -h 127.0.0.1 -uroot -p -e \"FLUSH PRIVILEGES ;\"\n            fi\n          fi\n          # 确定binlog 克隆数据位置(如果binlog存在的话).\n          cd /var/lib/mysql\n          # 如果存在该文件，则该xrabackup是从现有的从节点克隆出来的。\n          if [[ -s xtrabackup_slave_info ]]; then\n            mv xtrabackup_slave_info change_master_to.sql.in\n            rm -f xtrabackup_binlog_info\n          elif [[ -f xtrabackup_binlog_info ]]; then\n            [[ `cat xtrabackup_binlog_info` =~ ^(.*?)[[:space:]]+(.*?)$ ]] || exit 1\n            rm xtrabackup_binlog_info\n            echo \"CHANGE MASTER TO MASTER_LOG_FILE='${BASH_REMATCH[1]}',\\\n                  MASTER_LOG_POS=${BASH_REMATCH[2]}\" > change_master_to.sql.in\n          fi\n          if [[ -f change_master_to.sql.in ]]; then\n            echo \"Waiting for mysqld to be ready (accepting connections)\"\n            until mysql -h 127.0.0.1 -uroot --password=\"\" -e \"SELECT 1\"; do sleep 1; done\n            echo \"Initializing replication from clone position\"\n            mv change_master_to.sql.in change_master_to.sql.orig\n            sed -i \"s/;//\" change_master_to.sql.orig\n            mysql -h 127.0.0.1  -uroot --password=\"\" <<EOF\n            reset slave;\n          $(<change_master_to.sql.orig),\n            MASTER_HOST='mysql-0.mysql',\n            MASTER_USER='repl',\n            MASTER_PASSWORD='repl',\n            MASTER_CONNECT_RETRY=10;\n          START SLAVE;\nEOF\n          fi\n          exec ncat --listen --keep-open --send-only --max-conns=1 3307 -c \\\n            \"xtrabackup --backup --slave-info --stream=xbstream --host=127.0.0.1 --user=root --password=\"",
							},
							VolumeMounts: []v14.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/mysql",
									SubPath:   "mysql",
								},
								{
									Name:      "conf",
									MountPath: "/etc/mysql/conf.d",
								},
							},
							Resources: v14.ResourceRequirements{
								Requests: v14.ResourceList{
									v14.ResourceCPU:    resource.MustParse("10m"),
									v14.ResourceMemory: resource.MustParse("10Mi"),
								},
							},
						},
					},
					Volumes: []v14.Volume{
						{
							Name: "conf",
							VolumeSource: v14.VolumeSource{
								EmptyDir: &v14.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "config-map",
							VolumeSource: v14.VolumeSource{
								ConfigMap: &v14.ConfigMapVolumeSource{
									LocalObjectReference: v14.LocalObjectReference{
										Name: cr.Name,
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []v14.PersistentVolumeClaim{
				{
					ObjectMeta: v12.ObjectMeta{
						Name:      "data",
						Namespace: cr.Namespace,
					},
					Spec: v14.PersistentVolumeClaimSpec{
						AccessModes: []v14.PersistentVolumeAccessMode{
							"ReadWriteOnce",
						},
						StorageClassName: &scn,
						Resources: v14.ResourceRequirements{
							Requests: v14.ResourceList{
								v14.ResourceStorage: resource.MustParse("200Gi"),
							},
						},
					},
				},
			},
		},
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *MysqlReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&publicappv1.Mysql{}).
		Owns(&v1.StatefulSet{}).
		Owns(&v14.Service{}).
		Owns(&v14.ConfigMap{}).
		Complete(r)
}
