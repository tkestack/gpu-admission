Name: gpu-admission
Version: %{version}
Release: %{commit}%{?dist}
Summary: GPU admission

Group: Development/TKE
License: MIT
Source: gpu-admission-source.tar.gz

Requires: systemd-units

%define pkgname  %{name}-%{version}-%{release}

%description
GPU quota admission

%prep
%setup -n gpu-admission-%{version}
cat << EOF >> .version-defs
API_PKG_NAME='%{pkgname}'
EOF

%build
make all

%install
install -d $RPM_BUILD_ROOT/%{_bindir}
install -d $RPM_BUILD_ROOT/%{_unitdir}
install -d $RPM_BUILD_ROOT/etc/kubernetes

install -p -m 755 ./bin/gpu-admission $RPM_BUILD_ROOT/%{_bindir}/gpu-admission
install -p -m 644 ./build/gpu-admission.conf $RPM_BUILD_ROOT/etc/kubernetes/gpu-admission.conf
install -p -m 644 ./build/gpu-admission.service $RPM_BUILD_ROOT/%{_unitdir}/

%clean
rm -rf $RPM_BUILD_ROOT

%files
%config(noreplace,missingok) /etc/kubernetes/gpu-admission.conf

/%{_unitdir}/gpu-admission.service

/%{_bindir}/gpu-admission
